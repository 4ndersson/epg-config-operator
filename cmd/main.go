/*
Copyright 2024.

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
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	epgv1alpha1 "github.com/4ndersson/epg-config-operator/api/v1alpha1"
	"github.com/4ndersson/epg-config-operator/internal/controller"
	"github.com/4ndersson/epg-config-operator/pkg/aci"
	"github.com/tidwall/gjson"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(epgv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func getStartupConfiguration(c client.Client, cs *kubernetes.Clientset, co *rest.Config) (controller.CniConfig, error) {
	configConfigMap := &corev1.ConfigMapList{}
	err := c.List(context.TODO(),
		configConfigMap,
		client.InNamespace("aci-containers-system"),
		client.MatchingFields{"metadata.name": "aci-containers-config"})
	if err != nil {
		return controller.CniConfig{}, err
	}

	contractConfigMap := &corev1.ConfigMapList{}
	err = c.List(context.TODO(),
		contractConfigMap,
		client.InNamespace("aci-containers-system"),
		client.MatchingFields{"metadata.name": "default-epg-contracts"})
	if err != nil {
		return controller.CniConfig{}, err
	}

	podList := &corev1.PodList{}
	err = c.List(context.TODO(), podList, client.InNamespace("aci-containers-system"))

	if err != nil {
		return controller.CniConfig{}, err
	}

	controllerPod := ""
	for _, pod := range podList.Items {
		if strings.Contains(pod.Name, "controller") {
			controllerPod = pod.Name
			break
		}
	}

	var cert, password string

	if controllerPod != "" {
		req := cs.CoreV1().RESTClient().
			Post().
			Resource("pods").
			Name(controllerPod).
			Namespace("aci-containers-system").
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Command: []string{"/bin/sh", "-c",
					fmt.Sprintf("cat %s", gjson.Get(configConfigMap.Items[0].Data["controller-config"],
						"apic-private-key-path").String())},
				Stdin:  false,
				Stdout: true,
				Stderr: true,
			}, runtime.NewParameterCodec(scheme))

		exec, err := remotecommand.NewSPDYExecutor(co, "POST", req.URL())
		if err != nil {
			return controller.CniConfig{}, err
		}

		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			return controller.CniConfig{}, err
		}
		cert = stdout.String()
	} else {
		password = os.Getenv("APIC_PASSWORD")
	}

	if cert == "" && password == "" {
		return controller.CniConfig{}, fmt.Errorf("could not find cert or password")
	}

	providedContracts := gjson.Get(contractConfigMap.Items[0].Data["provided"], "@this").Array()
	providedContractsList := make([]string, len(providedContracts))
	for i, item := range providedContracts {
		providedContractsList[i] = item.String()
	}

	consumedContracts := gjson.Get(contractConfigMap.Items[0].Data["consumed"], "@this").Array()
	consumedContractsList := make([]string, len(consumedContracts))
	for i, item := range consumedContracts {
		consumedContractsList[i] = item.String()
	}

	return controller.CniConfig{
		ApicIp:         gjson.Get(configConfigMap.Items[0].Data["controller-config"], "apic-hosts.0").String(),
		ApicUsername:   gjson.Get(configConfigMap.Items[0].Data["controller-config"], "apic-username").String(),
		ApicPassword:   password,
		ApicPrivateKey: cert,
		KeyPath:        gjson.Get(configConfigMap.Items[0].Data["controller-config"], "apic-private-key-path").String(),
		Tenant:         gjson.Get(configConfigMap.Items[0].Data["controller-config"], "aci-policy-tenant").String(),
		BridgeDomain: strings.Replace(strings.Split(gjson.Get(configConfigMap.Items[0].Data["controller-config"],
			"aci-podbd-dn").String(), "/")[2], "BD-", "", -1),
		VmmDomain:          gjson.Get(configConfigMap.Items[0].Data["controller-config"], "aci-vmm-domain").String(),
		VmmDomainType:      gjson.Get(configConfigMap.Items[0].Data["controller-config"], "aci-vmm-type").String(),
		ApplicationProfile: gjson.Get(configConfigMap.Items[0].Data["controller-config"], "app-profile").String(),
		ProvidedContracts:  providedContractsList,
		ConsumedContracts:  consumedContractsList,
	}, nil
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization

		// TODO(user): If CertDir, CertName, and KeyName are not specified, controller-runtime will automatically
		// generate self-signed certificates for the metrics server. While convenient for development and testing,
		// this setup is not recommended for production.
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "327369c9.custom.aci",
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{&corev1.ConfigMap{}, &corev1.Pod{}},
			},
		},
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	config := mgr.GetConfig()
	client := mgr.GetClient()
	clientset, _ := kubernetes.NewForConfig(config)

	cniConfig, err := getStartupConfiguration(client, clientset, config)
	if err != nil {
		setupLog.Error(err, "unable to get startup configuration")
		os.Exit(1)
	}

	apicClient, err := aci.NewClient(cniConfig.ApicIp,
		cniConfig.ApicUsername,
		cniConfig.ApicPassword,
		cniConfig.ApicPrivateKey)
	if err != nil {
		setupLog.Error(err, "unable to setup apic client")
		os.Exit(1)
	}

	if err = (&controller.EpgconfReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		CniConfig:  cniConfig,
		ApicClient: apicClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Conf")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
