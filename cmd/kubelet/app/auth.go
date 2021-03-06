/*
Copyright 2016 The Kubernetes Authors.

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

package app

import (
	"errors"
	"fmt"
	"reflect"

	"k8s.io/kubernetes/pkg/apis/componentconfig"
	apiserverauthenticator "k8s.io/kubernetes/pkg/apiserver/authenticator"
	"k8s.io/kubernetes/pkg/auth/authenticator"
	"k8s.io/kubernetes/pkg/auth/authorizer"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	authenticationclient "k8s.io/kubernetes/pkg/client/clientset_generated/clientset/typed/authentication/v1beta1"
	authorizationclient "k8s.io/kubernetes/pkg/client/clientset_generated/clientset/typed/authorization/v1beta1"
	alwaysallowauthorizer "k8s.io/kubernetes/pkg/genericapiserver/authorizer"
	apiserverauthorizer "k8s.io/kubernetes/pkg/genericapiserver/authorizer"
	"k8s.io/kubernetes/pkg/kubelet/server"
	"k8s.io/kubernetes/pkg/types"
)

func buildAuth(nodeName types.NodeName, client clientset.Interface, config componentconfig.KubeletConfiguration) (server.AuthInterface, error) {
	// Get clients, if provided
	var (
		tokenClient authenticationclient.TokenReviewInterface
		sarClient   authorizationclient.SubjectAccessReviewInterface
	)
	if client != nil && !reflect.ValueOf(client).IsNil() {
		tokenClient = client.Authentication().TokenReviews()
		sarClient = client.Authorization().SubjectAccessReviews()
	}

	authenticator, err := buildAuthn(tokenClient, config.Authentication)
	if err != nil {
		return nil, err
	}

	attributes := server.NewNodeAuthorizerAttributesGetter(nodeName)

	authorizer, err := buildAuthz(sarClient, config.Authorization)
	if err != nil {
		return nil, err
	}

	return server.NewKubeletAuth(authenticator, attributes, authorizer), nil
}

func buildAuthn(client authenticationclient.TokenReviewInterface, authn componentconfig.KubeletAuthentication) (authenticator.Request, error) {
	authenticatorConfig := apiserverauthenticator.DelegatingAuthenticatorConfig{
		Anonymous:    authn.Anonymous.Enabled,
		CacheTTL:     authn.Webhook.CacheTTL.Duration,
		ClientCAFile: authn.X509.ClientCAFile,
	}

	if authn.Webhook.Enabled {
		if client == nil {
			return nil, errors.New("no client provided, cannot use webhook authentication")
		}
		authenticatorConfig.TokenAccessReviewClient = client
	}

	authenticator, _, err := authenticatorConfig.New()
	return authenticator, err
}

func buildAuthz(client authorizationclient.SubjectAccessReviewInterface, authz componentconfig.KubeletAuthorization) (authorizer.Authorizer, error) {
	switch authz.Mode {
	case componentconfig.KubeletAuthorizationModeAlwaysAllow:
		return alwaysallowauthorizer.NewAlwaysAllowAuthorizer(), nil

	case componentconfig.KubeletAuthorizationModeWebhook:
		if client == nil {
			return nil, errors.New("no client provided, cannot use webhook authorization")
		}
		authorizerConfig := apiserverauthorizer.DelegatingAuthorizerConfig{
			SubjectAccessReviewClient: client,
			AllowCacheTTL:             authz.Webhook.CacheAuthorizedTTL.Duration,
			DenyCacheTTL:              authz.Webhook.CacheUnauthorizedTTL.Duration,
		}
		return authorizerConfig.New()

	case "":
		return nil, fmt.Errorf("No authorization mode specified")

	default:
		return nil, fmt.Errorf("Unknown authorization mode %s", authz.Mode)

	}
}
