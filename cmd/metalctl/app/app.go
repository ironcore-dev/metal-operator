package app

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	metalv1alphav1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

const Name string = "metalctl"

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(metalv1alphav1.AddToScheme(scheme))
}

func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   Name,
		Short: "CLI client for metal-operator",
		Args:  cobra.NoArgs,
	}
	root.AddCommand(NewMoveCommand())
	return root
}

func GetKubeconfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
}
