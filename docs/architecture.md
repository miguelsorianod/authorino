# Authorino architecture and Feature description
- [Architecture](#architecture)
  - [Protecting upstream APIs with Envoy and Authorino](#protecting-upstream-apis-with-envoy-and-authorino)
  - [The Authorino `Service` Custom Resource Definition (CRD)](#the-authorino-service-custom-resource-definition-crd)
  - [The "Auth Pipeline"](#the-auth-pipeline)
  - [The Authorization JSON](#the-authorization-json)
- [Feature description](#feature-description)
  - [API key authentication](#api-key-authentication)
  - [Kubernetes authentication](#kubernetes-authentication)
  - [OpenID Connect (OIDC) JWT verification and validation](#openid-connect-oidc-jwt-verification-and-validation)
  - [OAuth 2.0 (token introspection)](#oauth-20-token-introspection)
  - [Festival Wristband authentication](#festival-wristband-authentication)
  - [Mutual Transport Layer Security (mTLS) authentication](#mutual-transport-layer-security-mtls-authentication)
  - [Hash Message Authentication Code (HMAC) authentication](#hash-message-authentication-code-hmac-authentication)
  - [OpenID Connect (OIDC) User Info](#openid-connect-oidc-user-info)
  - [User-Managed Access (UMA) resource data](#user-managed-access-uma-resource-data)
  - [External HTTP authorization metadata](#external-http-authorization-metadata)
  - [Open Policy Agent (OPA) Rego policies](#open-policy-agent-opa-rego-policies)
  - [Festival Wristbands](#festival-wristbands)

## Architecture

### Protecting upstream APIs with Envoy and Authorino

Typically, upstream APIs are deployed to the same Kubernetes cluster and namespace where the Envoy proxy and Authorino is running (although not necessarily). Whatever is the case, Envoy proxy must be serving the upstream API (see Envoy's [HTTP route components](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto) and virtual hosts) and pointing to Authorino in the external authorization filter.

An `Ingress` resource exposing Envoy service on a host name that must resolves to the Kubernetes cluster and identifies the API for external users. This host name is important as well for Authorino as it goes in the Authorino `Service` custom resource that declares the protection to the API.

You must then be ready to write and apply the custom resource that describes the desired state of the protection for the API.

There is no need to redeploy neither Authorino nor Envoy after applying a protection config. Authorino controller will automatically detect any changes relative to `config.authorino.3scale.net`/`Service` resources and reconcile them inside the running instances.

### The Authorino `Service` Custom Resource Definition (CRD)

Each API protection is declared in the form a [Kubernetes Custom Resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources).

Please check out the [spec](/config/crd/bases/config.authorino.3scale.net_services.yaml) of Authorino's `Service` custom resource for the details of how to configure **identity** verification, external authorization **metadata**, **authorization** policies and optional **wristband** token issuing on requests to you API.

A typical Authorino's `Service` custom resource looks like the following:

```yaml
apiVersion: config.authorino.3scale.net/v1beta1
kind: Service
metadata:
  name: my-api-protection
spec:
  hosts:
    - my-api.io # used for lookup

  # List of identity sources - at least one must accept the supplied token or credential
  identity:
    - name: my-idp-users
      oidc: # the authentication protocol (supported: apiKey, kubernetes, oidc, oauth2, mtls, hmac)
        issuer: https://my-idp/realm
      credentials: # where the access token of credential must travel in the request
        in: authorization_header
        keySelector: Bearer

  # List of external sources of authorization metadata (if you need any)
  metadata:
    - name: online-userinfo
      userInfo: # the external metadata protocol (supported: userInfo, uma, http)
        identitySource: my-idp-users

  # List of authorization policies – all policies must grant access
  authorization:
    - name: my-rbac-policy
      json: # the authorization policy format (supported: json, opa)
        conditions: # allows to skip this policy if the conditions do not match
          - selector: auth.identity.roles # fetched from the authorization JSON
            operator: excl # supported operator: eq, neq, incl, excl, matches
            value: admin
        rules: # the actual set of rules that must match when this policy is evaluates
          - selector: auth.content.request.method
            operator: eq
            value: GET

  # Festival Wristband (only if you want JWTs issued by Authorino at the end of the auth pipeline)
  wristband:
    issuer: http://authorino.svc.cluster.local:8003/namespace/my-api-protection
    customClaims:
      - name: foo
        value: bar
      - name: exp
        valueFrom:
          authJSON: auth.identity.exp
    signingKeyRefs:
      - name: my-signing-key
        algorithm: ES256
```

More concrete examples focused on each individual supported feature can be found at the [Examples page](/examples).

### The "Auth Pipeline"

![Authorino Auth Pipeline](auth-pipeline.png)

In each request to the protected API, Authorino triggers the so-called "Auth Pipeline", a set of configured *evaluators* that are organized in a 4-phase pipeline:

- **(i) Identity phase:** at least one source of identity (i.e., one identity evaluator) must resolve the supplied credential in the request into a valid identity or Authorino will otherwise reject the request as unauthenticated (401 HTTP response status).
- **(ii) Metadata phase:** optional fetching of additional data from external sources, to add up to context and identity information, and used in authorization policies and wristband claims (phases iii and iv).
- **(iii) Authorization phase:** all unskipped policies must evaluate to a positive result ("authorized"), or Authorino will otherwise reject the request as unauthorized (403 HTTP response code).
- **(iv) Wristband phase:** single evaluator phase where Authorino optionally issues an OIDC-compliant JSON Web Token (JWT) called "Festival Wristband" token, used for token normalization and/or as part of an Edge Authentication Architecture (EAA).

The evaluators set within each of the 3 first phases are triggered concurrently to each other, while the overall 4 phases are sequential. The **Identity** phase (i) is the only one required to list at least one evaluator (i.e. one identity source or more); **Metadata** and **Authorization** phases (respectively, ii and iii) can have any number of evaluators (including zero, and even be omitted in this case). The **Wristband** phase is a single-evaluator phase, and it can also be omitted if no wristband token should be issued at all.

### The Authorization JSON

Along the auth pipeline, Authorino builds the *authorization payload*, a JSON content composed of *context* information about the request, as provided by the proxy to Authorino, plus *auth* objects resolved and collected along of phases (i) and (ii). In each phase, the authorization JSON can be accessed by the evaluators, leading to phase (iii) counting with a payload (input) that looks like the following:

```jsonc
// The authorization JSON combined along Authorino's auth pipeline for each request
{
  "context": { // the input from the proxy
    "origin": {…},
    "request": {
      "http": {
        "method": "…",
        "headers": {…},
        "path": "/…",
        "host": "…",
        …
      }
    }
  },
  "auth": {
    "identity": {
      // the identity resolved, from the supplied credentials, by one of the evaluators of phase (i)
    },
    "metadata": {
      // each metadata object/collection resolved by the evaluators of phase (ii)
    }
  }
}
```

The policies evaluated in phase (iii) can use any data from the authorization JSON to define authorization rules. Festival Wristbands can also include custom claims fetching values from the authorization JSON.

## Feature description

This section is not an exhaustive list of features of Authorino. Rather, it describes implementation details of some of Authorino's most important features. For an updated list of all features and current state of development of each feature, please refer to the [List of features](/README.md#list-of-features) in the main page of the repo.

### API key authentication

Authorino relies on Kubernetes `Secret` resources to represent API keys. To define an API key, create a `Secret` in the cluster containing an `api_key` entry that holds the value of the API key. The resource must also include the same `labels` listed in the Authorino `Service` custom resource for the protected API that accepts the API key as a valid credential. For example:

For the following custom resource:

```yaml
apiVersion: config.authorino.3scale.net/v1beta1
kind: Service
metadata:
  name: my-api-protection
spec:
  hosts:
    - my-api.io
  identity:
    - name: api-key-users
      apiKey:
        labelSelectors: # the `labelSelectors` key-value set must match the metadata `labels` of the Secret resources
          authorino.3scale.net/managed-by: authorino # standard label to be added to the Secret (required)
          group: friends # custom label (optional)
```

The following secret would represent a valid API key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: user-1-api-key-1
  labels:
    authorino.3scale.net/managed-by: authorino
    group: friends
stringData:
  api_key: <some-randomly-generated-api-key-value>
type: Opaque
```

The resolved identity object, added to the authorization JSON following an API key identity source evaluation, is the Kubernetes `Secret` resource (as JSON).

### Kubernetes authentication

Authorino can verify Kubernetes-valid access tokens (using Kubernetes [TokenReview API](https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/)).

These tokens can be either `ServiceAccount` tokens such as the ones issued by kubelet as part of Kubernetes [Service Account Token Volume Projection](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-token-volume-projection), or any valid user access tokens issued to users of the Kubernetes server API.

The list of `audiences` of the token must include the requested host of the protected API (default), or all the audiences specified in the Authorino `Service` custom resource. For example:

For the following custom resource, the Kubernetes token must include the audience **my-api.io**:

```yaml
apiVersion: config.authorino.3scale.net/v1beta1
kind: Service
metadata:
  name: my-api-protection
spec:
  hosts:
    - my-api.io
  identity:
    - name: cluster-users
      kubernetes: {}
```

Whereas for the following custom resource, the Kubernetes token audiences must include **foo** and **bar**:

```yaml
apiVersion: config.authorino.3scale.net/v1beta1
kind: Service
metadata:
  name: my-api-protection
spec:
  hosts:
    - my-api.io
  identity:
    - name: cluster-users
      kubernetes:
        audiences:
          - foo
          - bar
```

The resolved identity object, added to the authorization JSON following a Kubernetes authentication identity source evaluation, is the decoded JWT when the Kubernetes token is a valid JWT, or the value of `status.user` in the response to the TokenReview request (see Kubernetes [UserInfo](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#userinfo-v1-authentication-k8s-io) for details).

### OpenID Connect (OIDC) JWT verification and validation

Authorino automatically discovers OpenID Connect configurations for the configured issuers and verifies JSON Web Tokens (JWTs) supplied on each request.

Authorino also fetches the JSON Web Key Sets (JWKS) used to verify the JWTs, matching the `kid` stated in the JWT header (i.e., support to key rotation).

![OIDC](http://www.plantuml.com/plantuml/png/XO_1IWD138RlynIX9mLt7s1XfQANseDGnPx7sMmtE9EqcOpQjtUeWego7aF-__lubzcyMadHvMVYlLUV80bBc5GIWcb1v_eUDXY40qNoHiADKNtslRigDeaI2pINiBXRtLp3AkU2ke0EJkT0ESWBwj7zV3UryDNkO8inDckMLuPg6cddM0mXucWT11ycd9TjyF0X3AYM_v7TRjVtl_ckRTlFiOU2sVvU-PtpY4hZiU8U8DEElHN5cRIFD7Z3K_uCt_ONm4_ZkLiY3oN5Tm00)

The decoded JWTs (and fetched user info) are appended to the authorization JSON as the resolved identity.

### OAuth 2.0 (token introspection)

For bare OAuth 2.0 implementations, Authorino can perform token introspection on the access tokens supplied in the requests to protected APIs.

Authorino does not implement any of OAuth 2.0 grants for the applications to obtain the token. However, it can verify supplied tokens with the OAuth server, including opaque tokens, as long as the server exposes the `token_introspect` endpoint ([RFC 7662](https://tools.ietf.org/html/rfc7662)).

Developers must set the token introspection endpoint in the [CR spec](#the-authorino-service-custom-resource-definition-crd), as well as a reference to the Kubernetes secret storing the credentials of the OAuth client to be used by Authorino when requesting the introspect.

![OAuth 2.0 Token Introspect](http://www.plantuml.com/plantuml/png/NP1DJiD038NtSmehQuQgsr4R5TZ0gXLaHwHgD779g8aTF1xAZv0u3GVZ9BHH18YbttkodxzLKY-Q-ywaVQJ1Y--XP-BG2lS8AXcDkRSbN6HjMIAnWrjyp9ZK_4Xmz8lrQOI4yeHIW8CRKk4qO51GtYCPOaMG-D2gWytwhe9P_8rSLzLcDZ-VrtJ5f4XggvS17VXXw6Bm6fbcp_PmEDWTIs-pT4Y16sngccgyZY47b-W51HQJRqCNJ-k2O9FAcceQsomNsgBr8M1ATbJAoTdgyV2sZQJBHKueji5T96nAy-z5-vSAE7Y38gbNBDo8xGo-FZxXtQoGcYFVRm00)

The response returned by the OAuth2 server to the token introspection request is the the resolved identity appended to the authorization JSON.

### Festival Wristband authentication

Authorino-issued [Festival Wristband](#festival-wristbands) tokens are valid OpenID Connect ID tokens (JWTs). To verify and validate Authorino Wristband tokens, use Authorino [OIDC authentication](#openid-connect-oidc-jwt-verification-and-validation). The value of the issuer must be the same issuer specified in the custom resource for the protected API originally issuing wristband (eventually, the same custom resource where the wristband is configured as a valid source of identity).

### Mutual Transport Layer Security (mTLS) authentication

`[Not Implemented]` Authentication based on client X509 certificates presented on the request to the protected APIs.

### Hash Message Authentication Code (HMAC) authentication

`[Not Implemented]` Authentication based on the validation of a hash code generated from the contextual information of the request to the protected API, concatenated with a secret known by the API consumer.

### OpenID Connect (OIDC) User Info

Online fetching of OpenID Connect (OIDC) UserInfo data (phase ii of the Authorino [Auth Pipeline](#the-auth-pipeline)), associated with an OIDC identity source configured and resolved in phase (i).

The response returned by the OIDC server to the UserInfo request is appended (as JSON) to `auth.metadata` in the authorization JSON.

### User-Managed Access (UMA) resource data

User-Managed Access (UMA) is an OAuth-based protocol for resource owners to allow other users to access their resources. Since the UMA-compliant server is expected to know about the resources, Authorino includes a client that fetches resource data from the server and adds that as metadata of the authorization payload.

This enables the implementation of resource-level Attribute-Based Access Control (ABAC) policies. Attributes of the resource fetched in a UMA flow can be, e.g., the owner of the resource, or any business-level attributes stored in the UMA-compliant server.

A UMA-compliant server is an external authorization server (e.g., Keycloak) where the protected resources are registered. It can be as well the upstream API itself, as long as it implements the UMA protocol, with initial authentication by `client_credentials` grant to exchange for a Protected API Token (PAT).

![UMA](http://www.plantuml.com/plantuml/png/ZOx1IWCn48RlUOgX9pri7w0GxOBYGGGj5G_Y8QHJTp2PgPE9qhStmhBW9NWSvll__ziM2ser9rS-Y4z1GuOiB75IoGYc5Ptp7dOOXICb2aR2Wr5xUk_6QfCeiS1m1QldXn4AwXVg2ZRmUzrGYTBki_lp71gzH1lwWYaDzopV357uIE-EnH0I7cq3CSG9dLklrxF9PyLY_rAOMNWSzts11dIBdYhg6HIBL8rOuEAwAlbJiEcoN_pQj9VOMtVZxdQ_BFHBTpC5Xs31RP4FDQSV)

It's important to notice that Authorino does NOT manage resources in the UMA-compliant server. As shown in the flow above, Authorino's UMA client is only to fetch data about the requested resources. Authorino exchanges client credentials for a Protected API Token (PAT), then queries for resources whose URI match the path of the HTTP request (as passed to Authorino by the Envoy proxy) and fetches data of each macthing resource.

The resources data is added as metadata of the authorization payload and passed as input for the configured authorization policies. All resources returned by the UMA-compliant server in the query by URI are passed along. They are available in the PDPs (authorization payload) as `input.auth.metadata.custom-name => Array`. (See [The "Auth Pipeline"](#the-auth-pipeline-aka-authorinos-3-core-phases) for details.)

### External HTTP authorization metadata

Generic HTTP adapter to fetch external metadata for the authorization policies (phase ii of the Authorino [Auth Pipeline](#the-auth-pipeline)).

The adapter allows fecthing auth metadata from external HTTP services by GET or POST requests. When POST is used, the [authorization JSON](#the-authorization-json) is passed in the body of the request.

A shared secret between Authorino and the external HTTP service must be defined (`sharedSecretRef` property), and the  service can use such secret to authenticate the origin of the request. The location where the secret travels in the request performed by Authorino to the HTTP service can be specified in a typical "credentials" property.

### Open Policy Agent (OPA) Rego policies

You can model authorization policies in [Rego language](https://www.openpolicyagent.org/docs/latest/policy-language/) and add them as part of the protection of your APIs. Authorino reconciliation cycle keeps track of any changes in the custom resources affecting the written policies and automatically recompiles them with built-in OPA module, and cache them for fast evaluation during request-time.

![OPA](http://www.plantuml.com/plantuml/png/ZSv1IiH048NXVPsYc7tYVY0oeuYB0IFUeEcKfh2xAdPUATxUB4GG5B9_F-yxhKWDKGkjhsfBQgboTVCyDw_2Q254my1Fajso5arGjmvQXOU1pe7PcvfpTys7cz22Jet7n_E1ZrlqeYkayU95yoVz7loPt7fTjCX_nPRyN98vX8iyuyWvvLc8-hx_rhw5hDZ9l1Vmv3cg6FX3CRFQ4jZ3lNjF9H9sURVvUBbw62zq4fkYbYy0)

### Festival Wristbands

Festival Wristbands are OpenID Connect JSON Web Tokens (JWTs) issued by Authorino at the end of the auth pipeline and passed back to the client in the HTTP response header `X-Ext-Auth-Wristband`. It is an opt-in feature that can be used to enable Edge Authentication and token normalization. Authorino wristbands can also be used as vessels to carry from the external authorization back to the client (with custom claims).

The Authorino `Service` custom resource below sets an API protection that issues a wristband after a successful authentication via API key. The wristband contains standard JWT claims such as `iss`, `iat`, and `exp` and 2 custom claims: a static value `aud=internal` and a dynamic value `born` that fetches from the authorization JSON the creation timestamp of the secret that represents the API key used to authenticate.

```yaml
apiVersion: config.authorino.3scale.net/v1beta1
kind: Service
metadata:
  namespace: my-namespace
  name: my-api-protection
spec:
  hosts:
    - my-api.io
  identity:
    - name: edge
      apiKey:
        labelSelectors:
          authorino.3scale.net/managed-by: authorino
      credentials:
        in: authorization_header
        keySelector: APIKEY
  wristband:
    issuer: http://authorino.svc.cluster.local:8003/my-namespace/talker-api-protection
    customClaims:
      - name: aud
        value: internal
      - name: born
        valueFrom:
          authJSON: auth.identity.metadata.creationTimestamp
    tokenDuration: 300
    signingKeyRefs:
      - name: my-signing-key
        algorithm: ES256
      - name: my-old-signing-key
        algorithm: RS256
```

The signing key names listed in `signingKeyRefs` must match the names of Kubernetes `Secret` resources created in the same namespace, where each secret contains a `key.pem` entry that holds the value of the private key that will be used to sign the wristbands issued, formatted as [PEM](https://en.wikipedia.org/wiki/Privacy-Enhanced_Mail). The first key in this list will be used to sign the wristbands, while the others are kept to support key rotation.

For each protected API configured for the Festival Wristband issuing, Authorino exposes the following OpenID Connect Discovery well-known endpoints:
- **OpenID Connect configuration:**<br/>
  http://authorino.svc.cluster.local:8003/{namespace}/{api-protection-name}/.well-known/openid-configuration
- **JSON Web Key Set (JWKS) well-known endpoint:**<br/>
  http://authorino.svc.cluster.local:8003/{namespace}/{api-protection-name}/.well-known/openid-connect/certs
