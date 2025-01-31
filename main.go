/*
Copyright 2020 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"net"
	"net/http"
	"os"

	envoy_auth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	configv1beta1 "github.com/kuadrant/authorino/api/v1beta1"
	"github.com/kuadrant/authorino/controllers"
	"github.com/kuadrant/authorino/pkg/cache"
	"github.com/kuadrant/authorino/pkg/common"
	"github.com/kuadrant/authorino/pkg/service"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	GRPCMaxConcurrentStreams    = 10000
	authorinoWatchedSecretLabel = "authorino.3scale.net/managed-by"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(configv1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "cb88a58a.authorino.3scale.net",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	cache := cache.NewCache()

	// sets up the service reconciler
	serviceReconciler := &controllers.ServiceReconciler{
		Client: mgr.GetClient(),
		Cache:  &cache,
		Log:    ctrl.Log.WithName("authorino").WithName("controller").WithName("Service"),
		Scheme: mgr.GetScheme(),
	}
	if err = serviceReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
	}

	// sets up secret reconciler
	if err = (&controllers.SecretReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("authorino").WithName("controller").WithName("Secret"),
		Scheme:            mgr.GetScheme(),
		SecretLabel:       common.FetchEnv("AUTHORINO_SECRET_LABEL_KEY", authorinoWatchedSecretLabel),
		ServiceReconciler: serviceReconciler,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Secret")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	startExtAuthServer(&cache)
	startOIDCServer(&cache)

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func startExtAuthServer(serviceCache *cache.Cache) {
	startExtAuthServerGRPC(serviceCache)
	startExtAuthServerHTTP(serviceCache)
}

func startExtAuthServerGRPC(serviceCache *cache.Cache) {
	logger := ctrl.Log.WithName("authorino").WithName("auth")
	port := common.FetchEnv("EXT_AUTH_GRPC_PORT", "50051")

	if lis, err := net.Listen("tcp", ":"+port); err != nil {
		logger.Error(err, "failed to obtain port for grpc auth service")
		os.Exit(1)
	} else {
		opts := []grpc.ServerOption{grpc.MaxConcurrentStreams(GRPCMaxConcurrentStreams)}
		s := grpc.NewServer(opts...)

		envoy_auth.RegisterAuthorizationServer(s, &service.AuthService{Cache: serviceCache})
		healthpb.RegisterHealthServer(s, &service.HealthService{})

		logger.Info("starting grpc service", "port", port)

		go func() {
			if err := s.Serve(lis); err != nil {
				logger.Error(err, "failed to start grpc service")
				os.Exit(1)
			}
		}()
	}
}

func startExtAuthServerHTTP(serviceCache *cache.Cache) {
	// TODO
}

func startOIDCServer(serviceCache *cache.Cache) {
	logger := ctrl.Log.WithName("authorino").WithName("oidc")
	port := common.FetchEnv("OIDC_HTTP_PORT", "8003")

	if lis, err := net.Listen("tcp", ":"+port); err != nil {
		logger.Error(err, "failed to obtain port for http oidc service")
		os.Exit(1)
	} else {
		http.Handle("/", &service.OidcService{
			Cache: serviceCache,
		})

		logger.Info("starting oidc service", "port", port)

		go func() {
			if err := http.Serve(lis, nil); err != nil {
				logger.Error(err, "failed to start oidc service")
				os.Exit(1)
			}

			// TODO: ServeTLS
		}()
	}
}
