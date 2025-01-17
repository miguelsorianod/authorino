# Authorino

Cloud-native AuthN/AuthZ enforcer for Zero Trust API protection.

- **User authentication/identity verification**<br/>
  API key, OAuth2, OIDC, mTLS, HMAC, K8s-auth
- **Ad-hoc authorization metadata**<br/>
  OIDC UserInfo, UMA-protected resource data, HTTP GET-by-POST
- **Authorization policy enforcement**<br/>
  OPA/Rego policies, JSON/JWT pattern matching policies
- **Token normalization**<br/>
  Built-in OIDC "Festival Wristband" tokens

Authorino enables hybrid API security layer, with usually no code changes required, tailor-made for your combination of authentication standards and protocols and authorization policies of choice.

Authorino builds on top of [Envoy Proxy](https://www.envoyproxy.io) [external authorization](https://www.envoyproxy.io/docs/envoy/latest/start/sandboxes/ext_authz) gRPC protocol, and complies with Red Hat [Kuadrant](https://github.com/kuadrant) architecture.

## How it works

![How it works](http://www.plantuml.com/plantuml/png/TP71JiCm38RlUGghLyGBS42hq2Gc3bHeSFSQZqLRx68xeFBqP1negTJhztz9zF_PcADwoPTWvyG3AcR8mjAVL3A1Qw4GiBXmoGVIqBJT3dfXAvcqWTjxsFAdZ707Z_jz1qeVXSp3BxocRV6JQ6AfnljBzn0ci4ZVIDDnX1I9FVcuBhOnGYR7Y8xhrfQFeZq1LlODWrnIdyZM_PrP8VZIP7v0Zk1o9fxhiwhFJt0pgLFPvdKmLy8CTQLckSaYlLwuMFFlW0sLqRz7DiInVjCF)

1. An application client (_API consumer_) obtains credentials to consume resources of the _Upstream API_, and sends a request to the _Envoy_ exposed endpoint
2. The Envoy proxy establishes fast gRPC connection with _Authorino_ carrying data of the HTTP request (context info)
3. **Identity verification phase** - Authorino verifies the identity of the the consumer, where at least one authentication method/identity provider must thrive
4. **Ad-hoc authorization metadata phase** - Authorino integrates external sources of additional metadata (optional)
5. **Policy enforcement phase** - Authorino takes as input a JSON composed of context information, resolved identity and fetched additional metadata from previous phases, and triggers the evaluation of configured authorization policies
6. **Wristband phase** – Authorino issues the _Festival Wristband_ OIDC token (optional), with standard and custom claims (static and dynamic values supported), to implement token normalization and/or Edge Authentication Architecture (EAA).
7. Authorino and Envoy settle the authorization protocol with either OK/NOK response (plus extra details available in the `X-Ext-Auth-Reason` and `WWW-Authenticate` headers when NOK)
8. If authorized, Envoy redirects to the requested _Upstream API_
9. The _Upstream API_ serves the requested resource to the consumer

The core phases of Authorino [Auth Pipeline](docs/architecture.md#the-auth-pipeline) (depicted in the diagram as steps 3 to 6) rely on well-established industry standards and protocols, such as [OpenID Connect (OIDC)](https://openid.net/connect/), [User-Managed Access (UMA)](https://docs.kantarainitiative.org/uma/rec-uma-core.html), [Open Policy Agent (OPA)](https://www.openpolicyagent.org/), [mutual Transport Layer Security (mTLS)](https://www.rfc-editor.org/rfc/rfc8705.html), to enable API Zero Trust security, while allowing API developers to pick and combine protocols and settings into one cloud-native configuration (based on Kubernetes [Custom Resource Definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources)).

## List of features

<table>
  <thead>
    <tr>
      <th colspan="2">Feature</th>
      <th>Description</th>
      <th>Stage</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td rowspan="7">Identity verification</td>
      <td>API key</td>
      <td>Represented as Kubernetes `Secret` resources. The secret MUST contain an entry `api_key` that holds the value of the API key. The secret MUST also contain at least one lable `authorino.3scale.net/managed-by` with whatever value, plus any number of optional labels. The labels are used by Authorino to match corresponding API protections that accept the API key as valid credential.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>mTLS</td>
      <td>Authentication by client certificate.</td>
      <td>Planned (<a href="https://github.com/kuadrant/authorino/issues/8">#8</a>)</td>
    </tr>
    <tr>
      <td>HMAC</td>
      <td>Authentication by Hash Message Authentication Code (HMAC), where a unique secret generated per API consumer, combined with parts of the request metadata, is used to generate a hash that is passed as authentication value by the client and verified by Authorino.</td>
      <td>Planned (<a href="https://github.com/kuadrant/authorino/issues/9">#9</a>)</td>
    </tr>
    <tr>
      <td>OAuth 2.0 (token introspection)</td>
      <td>Online introspection of access tokens with an OAuth 2.0 server.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>OpenID Connect (OIDC)</td>
      <td>Offline signature verification and time validation of OpenID Connect ID tokens (JWTs). Authorino caches the OpenID Connect configuration and JSON Web Key Set (JWKS) obtained from the OIDC Discovery well-known endpoint, and uses them to verify and validate tokens in request time.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>Kubernetes auth</td>
      <td>Online verification of Kubernetes access tokens through the Kubernetes TokenReview API. The `audiences` of the token MUST include the ones specified in the API protection state, which, when omitted, is assumed to be equal to the host name of the protected API. It can be used to authenticate Kubernetes `Service Account`s (e.g. other pods running in the cluster) and users of the cluster in general.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>OpenShift OAuth (user-echo endpoint)</td>
      <td>Online token introspection of OpenShift-valid access tokens based on OpenShift's user-echo endpoint.</td>
      <td>In analysis</td>
    </tr>
    <tr>
      <td rowspan="3">Ad-hoc authorization metadata</td>
      <td>OIDC user info</td>
      <td>Online request to OpenID Connect User Info endpoint. Requires an associated OIDC identity source.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>UMA-protected resource attributes</td>
      <td>Online request to a User-Managed Access (UMA) server to fetch data from the UMA Resource Set API.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>External HTTP service</td>
      <td>Generic online HTTP request to an external service. It can be used to fetch online metadata for the auth pipeline or as a web hook.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td rowspan="4">Policy enforcement</td>
      <td>JSON pattern matching (e.g. JWT claims)</td>
      <td>Authorization policies represented as simple JSON pattern-matching rules. Values can be selected from the authorization JSON built along the auth pipeline. Operations include _equals_ (`eq`), _not equal_ (`neq`), _includes_ (`incl`, for arrays), _excludes_ (`excl`, for arrays) and _matches_ (`matches`, for regular expressions). Individuals policies can be optionally skipped based on "conditions" represented with similar data selectors and operators.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>OPA Rego policies</td>
      <td>Built-in evaluator of Open Policy Agent (OPA) inline Rego policies. The policies written in Rego language are compiled and cached by Authorino in reconciliation-time, and evaluated against the authorization JSON in every request.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>Keycloak (UMA-compliant Authorization API)</td>
      <td>Online delegation of authorization to a Keycloak server.</td>
      <td>In analysis</td>
    </tr>
    <tr>
      <td>HTTP external authorization service</td>
      <td>Generic online delegation of authorization to an external HTTP service.</td>
      <td>In analysis</td>
    </tr>
    <tr>
      <td rowspan="6">Caching</td>
      <td>OIDC and UMA configs</td>
      <td>OpenID Connect and User-Managed Access configurations discovered in reconciliation-time.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>JSON Web Keys (JWKs) and JSON Web Ket Sets (JWKS)</td>
      <td>JSON signature verification certificates discovered usually in reconciliation-time, following an OIDC discovery associated to an identity source.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>Revoked access tokens</td>
      <td>Caching of access tokens identified as revoked before expiration.</td>
      <td>In analysis (<a href="https://github.com/kuadrant/authorino/issues/19">#19</a>)</td>
    </tr>
    <tr>
      <td>Resource data</td>
      <td>Caching of resource data obtained in previous requests.</td>
      <td>Planned (<a href="https://github.com/kuadrant/authorino/issues/21">#21</a>)</td>
    </tr>
    <tr>
      <td>Compiled Rego policies</td>
      <td>Performed automatically by Authorino in reconciliation-time for the authorization policies based on the built-in OPA module.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td>Repeated requests</td>
      <td>For consecutive requests performed, within a given period of time, by a same user that request for a same resource, such that the result of the auth pipeline can be proven that would not change.</td>
      <td>In analysis (<a href="https://github.com/kuadrant/authorino/issues/20">#20</a>)</td>
    </tr>
    <tr>
      <td colspan="2">Festival Wristbands</td>
      <td>JWTs issued by Authorino at the end of the auth pipeline and passed back to the client in the HTTP response header `X-Ext-Auth-Wristband`. Opt-in feature that can be used to enable Edge Authentication and token normalization, as well as to carry data from the external authorization back to the client (with support to static and dynamic custom claims). Authorino also exposes well-known endpoints for OpenID Connect Discovery, so the wristbands can be verified and validated, including by Authorino itself using the OIDC identity verification feature.</td>
      <td>Ready</td>
    </tr>
    <tr>
      <td colspan="2">Multitenancy</td>
      <td>Managed instances of Authorino offered to API providers who create and maintain their own API protection states within their own realms and namespaces.</td>
      <td>Planned</td>
    </tr>
    <tr>
      <td colspan="2">External policy registry</td>
      <td>Fetching of compatible policies from an external registry, in reconciliation-time.</td>
      <td>In analysis</td>
    </tr>
  </tbody>
</table>

For a detailed description of the features above, refer to the [Architecture](/docs/architecture.md#feature-description) page.

## Architecture

The [Architecture](docs/architecture.md) section of the docs covers the details of [protecting your APIs](docs/architecture.md#protecting-upstream-apis-with-envoy-and-authorino) with Envoy and Authorino, including a description of the components involved and specification of the [Authorino `Service` Custom Resource Definition (CRD)](docs/architecture.md#the-authorino-service-custom-resource-definition-crd).

You will also find in that section information about the Authorino [Auth Pipeline](docs/architecture.md#the-auth-pipeline), and detailed [description of features](docs/architecture.md#feature-description).

## Usage

1. [Deploy](docs/deploy.md) Authorino to the Kubernetes server
2. Have your upstream API [ready](docs/architecture.md#protecting-upstream-apis-with-envoy-and-authorino) to be protected
3. [Write](docs/architecture.md#the-authorino-service-custom-resource-definition-crd) and apply a `config.authorino.3scale.net`/`Service` custom resource declaring the desired state of the protection of your API

## Examples and Tutorials

The [Examples](examples) page lists several use cases and demonstrates how to implement those as Authorino custom resources. Each example use case presents a feature of Authorino and is independent from the other.

The Authorino [Tutorials](docs/tutorials.md) provide guided examples for deploying and protecting an API with Authorino and the Envoy proxy, where each tutorial combines multiple features of Authorino into one cohesive use case, resembling real life use cases.

## Terminology

You can find definitions for terms used in this document and others in the [Terminology](docs/terminology.md) document.

## Contributing

If you are interested in contributing to Authorino, please refer to instructions available [here](docs/contributing.md). You may as weel check our [Code of Conduct](docs/code_of_conduct.md).
