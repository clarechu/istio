// Copyright Istio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package install

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"istio.io/istio/istioctl/pkg/clioptions"
	"istio.io/istio/istioctl/pkg/util/formatting"
	"istio.io/istio/istioctl/pkg/verifier"
	"istio.io/istio/operator/cmd/mesh"
	"istio.io/istio/pkg/config/constants"
)

<<<<<<< HEAD
var (
	istioOperatorGVR = apimachinery_schema.GroupVersionResource{
		Group:    v1alpha1.SchemeGroupVersion.Group,
		Version:  v1alpha1.SchemeGroupVersion.Version,
		Resource: "istiooperators",
	}
)

func verifyInstallIOPrevision(enableVerbose bool, istioNamespaceFlag string,
	restClientGetter genericclioptions.RESTClientGetter,
	writer io.Writer, opts clioptions.ControlPlaneOptions, manifestsPath string) error {

	iop, err := operatorFromCluster(istioNamespaceFlag, opts.Revision, restClientGetter)
	if err != nil {
		return fmt.Errorf("could not load IstioOperator from cluster: %v.  Use --filename", err)
	}
	if manifestsPath != "" {
		iop.Spec.InstallPackagePath = manifestsPath
	}
	crdCount, istioDeploymentCount, err := verifyPostInstallIstioOperator(enableVerbose,
		istioNamespaceFlag,
		iop,
		fmt.Sprintf("in cluster operator %s", iop.GetName()),
		restClientGetter,
		writer)
	return show(crdCount, istioDeploymentCount, err, writer)
}

func verifyInstall(enableVerbose bool, istioNamespaceFlag string,
	restClientGetter genericclioptions.RESTClientGetter, options resource.FilenameOptions,
	writer io.Writer) error {

	// This is not a pre-check.  Check that the supplied resources exist in the cluster
	r := resource.NewBuilder(restClientGetter).
		Unstructured().
		FilenameParam(false, &options).
		Flatten().
		Do()
	if r.Err() != nil {
		return r.Err()
	}
	visitor := genericclioptions.ResourceFinderForResult(r).Do()
	crdCount, istioDeploymentCount, err := verifyPostInstall(enableVerbose,
		istioNamespaceFlag,
		visitor,
		strings.Join(options.Filenames, ","),
		restClientGetter,
		writer)
	return show(crdCount, istioDeploymentCount, err, writer)
}

func show(crdCount, istioDeploymentCount int, err error, writer io.Writer) error {
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(writer, "Checked %v custom resource definitions\n", crdCount)
	_, _ = fmt.Fprintf(writer, "Checked %v Istio Deployments\n", istioDeploymentCount)
	if istioDeploymentCount == 0 {
		_, _ = fmt.Fprintf(writer, "No Istio installation found\n")
		return fmt.Errorf("no Istio installation found")
	}
	_, _ = fmt.Fprintf(writer, "Istio is installed successfully\n")
	return nil
}

func verifyPostInstall(enableVerbose bool, istioNamespaceFlag string,
	visitor resource.Visitor, filename string, restClientGetter genericclioptions.RESTClientGetter, writer io.Writer) (int, int, error) {
	crdCount := 0
	istioDeploymentCount := 0
	err := visitor.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(info.Object)
		if err != nil {
			return err
		}
		un := &unstructured.Unstructured{Object: content}
		kind := un.GetKind()
		name := un.GetName()
		namespace := un.GetNamespace()
		kinds := findResourceInSpec(kind)
		if kinds == "" {
			kinds = strings.ToLower(kind) + "s"
		}
		if namespace == "" {
			namespace = "default"
		}
		switch kind {
		case "Deployment":
			deployment := &appsv1.Deployment{}
			err = info.Client.
				Get().
				Resource(kinds).
				Namespace(namespace).
				Name(name).
				VersionedParams(&meta_v1.GetOptions{}, scheme.ParameterCodec).
				Do(context.TODO()).
				Into(deployment)
			if err != nil {
				return err
			}
			err = getDeploymentStatus(deployment, name, filename)
			if err != nil {
				return err
			}
			if namespace == istioNamespaceFlag && strings.HasPrefix(name, "istio-") {
				istioDeploymentCount++
			}
		case "Job":
			job := &v1batch.Job{}
			err = info.Client.
				Get().
				Resource(kinds).
				Namespace(namespace).
				Name(name).
				VersionedParams(&meta_v1.GetOptions{}, scheme.ParameterCodec).
				Do(context.TODO()).
				Into(job)
			if err != nil {
				return err
			}
			for _, c := range job.Status.Conditions {
				if c.Type == v1batch.JobFailed {
					msg := fmt.Sprintf("Istio installation failed, incomplete or"+
						" does not match \"%s\" - the required Job %s failed", filename, name)
					return errors.New(msg)
				}
			}
		case "IstioOperator":
			// It is not a problem if the cluster does not include the IstioOperator
			// we are checking.  Instead, verify the cluster has the things the
			// IstioOperator specifies it should have.

			// IstioOperator isn't part of pkg/config/schema/collections,
			// usual conversion not available.  Convert unstructured to string
			// and ask operator code to unmarshal.

			iop, err := convertToIstioOperator(un.Object)
			if err != nil {
				return err
			}
			generatedCrds, generatedDeployments, err := verifyPostInstallIstioOperator(enableVerbose, istioNamespaceFlag, iop, filename, restClientGetter, writer)
			if err != nil {
				return err
			}
			crdCount += generatedCrds
			istioDeploymentCount += generatedDeployments
		default:
			result := info.Client.
				Get().
				Resource(kinds).
				Name(name).
				Do(context.TODO())
			if result.Error() != nil {
				result = info.Client.
					Get().
					Resource(kinds).
					Namespace(namespace).
					Name(name).
					Do(context.TODO())
				if result.Error() != nil {
					msg := fmt.Sprintf("Istio installation failed, incomplete or"+
						" does not match \"%s\" - the required %s:%s is not ready due to: %v", filename, kind, name, result.Error())
					return errors.New(msg)
				}
			}
			if kind == "CustomResourceDefinition" {
				crdCount++
			}
		}
		if enableVerbose {
			_, _ = fmt.Fprintf(writer, "%s: %s.%s checked successfully\n", kind, name, namespace)
		}
		return nil
	})
	if err != nil {
		return crdCount, istioDeploymentCount, err
	}
	return crdCount, istioDeploymentCount, nil
}

// todo clare 用于验证的安装的命令
=======
>>>>>>> 05ba771af6cd839e06483c3157ad910cb664da07
// NewVerifyCommand creates a new command for verifying Istio Installation Status
func NewVerifyCommand() *cobra.Command {
	var (
		kubeConfigFlags = &genericclioptions.ConfigFlags{
			Context:    strPtr(""),
			Namespace:  strPtr(""),
			KubeConfig: strPtr(""),
		}

		filenames      = []string{}
		istioNamespace string
		opts           clioptions.ControlPlaneOptions
		manifestsPath  string
	)
	verifyInstallCmd := &cobra.Command{
		Use:   "verify-install [-f <deployment or istio operator file>] [--revision <revision>]",
		Short: "Verifies Istio Installation Status",
		Long: `
verify-install verifies Istio installation status against the installation file
you specified when you installed Istio. It loops through all the installation
resources defined in your installation file and reports whether all of them are
in ready status. It will report failure when any of them are not ready.

If you do not specify an installation it will check for an IstioOperator resource
and will verify if pods and services defined in it are present.

Note: For verifying whether your cluster is ready for Istio installation, see
istioctl experimental precheck.
`,
		Example: `  # Verify that Istio is installed correctly via Istio Operator
  istioctl verify-install

  # Verify the deployment matches a custom Istio deployment configuration
  istioctl verify-install -f $HOME/istio.yaml

  # Verify the deployment matches the Istio Operator deployment definition
  istioctl verify-install --revision <canary>

  # Verify the installation of specific revision
  istioctl verify-install -r 1-9-0`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(filenames) > 0 && opts.Revision != "" {
				cmd.Println(cmd.UsageString())
				return fmt.Errorf("supply either a file or revision, but not both")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			installationVerifier, err := verifier.NewStatusVerifier(istioNamespace, manifestsPath,
				*kubeConfigFlags.KubeConfig, *kubeConfigFlags.Context, filenames, opts)
			if err != nil {
				return err
			}
			if formatting.IstioctlColorDefault(c.OutOrStdout()) {
				installationVerifier.Colorize()
			}
			return installationVerifier.Verify()
		},
	}

	flags := verifyInstallCmd.PersistentFlags()
	flags.StringVarP(&istioNamespace, "istioNamespace", "i", constants.IstioSystemNamespace,
		"Istio system namespace")
	kubeConfigFlags.AddFlags(flags)
	flags.StringSliceVarP(&filenames, "filename", "f", filenames, "Istio YAML installation file.")
	verifyInstallCmd.PersistentFlags().StringVarP(&manifestsPath, "manifests", "d", "", mesh.ManifestsFlagHelpStr)
	opts.AttachControlPlaneFlags(verifyInstallCmd)
	return verifyInstallCmd
}

func strPtr(val string) *string {
	return &val
}
