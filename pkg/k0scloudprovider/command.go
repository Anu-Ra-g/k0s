/*
Copyright 2021 k0s authors

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

package k0scloudprovider

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/options"
	cliflag "k8s.io/component-base/cli/flag"
)

type Command func(stopCh <-chan struct{})

type Config struct {
	AddressCollector AddressCollector
	KubeConfig       string
	BindPort         int
	UpdateFrequency  time.Duration
}

// NewCommand creates a new k0s-cloud-provider based on a configuration.
// The command itself is a specialization of the sample code available from
// `k8s.io/cloud-provider/app`
func NewCommand(c Config) (Command, error) {
	ccmo, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize cloud provider command options: %w", err)
	}

	ccmo.KubeCloudShared.CloudProvider.Name = Name
	ccmo.Kubeconfig = c.KubeConfig

	if c.BindPort != 0 {
		ccmo.SecureServing.BindPort = c.BindPort
	}

	if c.UpdateFrequency != 0 {
		ccmo.NodeStatusUpdateFrequency = metav1.Duration{Duration: c.UpdateFrequency}
	}

	cloudInitializer := func(*config.CompletedConfig) cloudprovider.Interface {
		// Returns the "k0s cloud provider" using the specified `AddressCollector`
		return NewProvider(c.AddressCollector)
	}

	// K0s only supports the cloud-node controller, so only use that.
	initFuncConstructors := make(map[string]app.ControllerInitFuncConstructor)
	for _, name := range []string{"cloud-node"} {
		var ok bool
		initFuncConstructors[name], ok = app.DefaultInitFuncConstructors[name]
		if !ok {
			return nil, fmt.Errorf("failed to find cloud provider controller %q", name)
		}
	}

	additionalFlags := cliflag.NamedFlagSets{}

	return func(stopCh <-chan struct{}) {
		command := app.NewCloudControllerManagerCommand(ccmo, cloudInitializer, initFuncConstructors, additionalFlags, stopCh)

		// Override the commands arguments to avoid it by default using `os.Args[]`
		command.SetArgs([]string{})

		if err := command.Execute(); err != nil {
			logrus.WithError(err).Errorf("Failed to execute k0s cloud provider")
		}
	}, nil
}
