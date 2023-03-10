package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/typed/extensions/v1beta1"

	// "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeConfig := prepareKubeConfig()

	availableResources := []string{"IngressRoutes", "Ingresses"}
	fmt.Printf("\nChoose resource ID from the list:\nResource Name \t\tID\n")
	fmt.Print("---------------------   ---\n")
	for i := 0; i < len(availableResources); i++ {
		fmt.Printf("%s \t\t[%d]\n", availableResources[i], i)
	}
	var resource string
	fmt.Print("ID: ")
	fmt.Scanln(&resource)

	switch resource {
	case availableResources[0]:
		dynClient := createDynClient(kubeConfig)
		var resourceList []unstructured.Unstructured
		resourceList = createIngressRoutesList(context.Background(), dynClient, "")
		hosts := make(map[string]string, len(resourceList))
		ingressRoutesListProcessing(resourceList, hosts)

		csvFile := createCsv("IngressRoutes-DNS-IP.csv")
		writeToCsv(csvFile, hosts)

	case availableResources[1]:
		newClient, err := v1beta1.NewForConfig(kubeConfig)
		if err != nil {
			fmt.Printf("error creating new client: %v\n", err)
			os.Exit(1)
		}
		new := newClient.Ingresses("")
		list, err := new.List(context.Background(), v1.ListOptions{})
		if err != nil {
			fmt.Printf("error creating new list: %v\n", err)
			os.Exit(1)
		}
		hosts := make(map[string]string, len(list.Items))
		for i := 0; i < len(list.Items); i++ {
			rules := list.Items[i].Spec.Rules
			for j := 0; j < len(rules); j++ {
				resolved, _ := resolveDNS(rules[j].Host)
				hosts[rules[j].Host] = resolved
			}
		}

		csvFile := createCsv("Ingresses-DNS-IP.csv")
		writeToCsv(csvFile, hosts)
	default:
		fmt.Print("Bad ID, please rerun the program.")
	}

}

func prepareKubeConfig() *rest.Config {
	var kubeConfigName string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("error getting user home dir: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Enter kubeconfig file to use from the folder:\n%s/", filepath.Join(userHomeDir, ".kube"))
	fmt.Scanln(&kubeConfigName)

	kubeConfigPath := filepath.Join(userHomeDir, ".kube", kubeConfigName)
	fmt.Printf("Using kubeconfig: %s\n", kubeConfigPath)
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		fmt.Printf("error getting Kubernetes config: %v\n", err)
		os.Exit(1)
	}

	return kubeConfig
}

// Create a dynamic client for k8s
func createDynClient(config *rest.Config) dynamic.Interface {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("Cannot create dynamic interface: %v\n", err)
		os.Exit(1)
	}

	return dynClient
}

// Create a list with IngressRoutes
func createIngressRoutesList(ctx context.Context, client dynamic.Interface, namespace string) []unstructured.Unstructured {
	// point schema to use
	var ingressRouteResource = schema.GroupVersionResource{Group: "traefik.containo.us", Version: "v1alpha1", Resource: "ingressroutes"}
	// GET /apis/traefik.containo.us/v1alpha1/namespaces/{namespace}/ingressroutes/
	list, err := client.Resource(ingressRouteResource).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		fmt.Printf("Cannot create ingressroutes list: %v\n", err)
		os.Exit(1)
	}

	return list.Items
}

// Start processing the list
func ingressRoutesListProcessing(list []unstructured.Unstructured, outList map[string]string) {
	for _, ingressRoute := range list {
		// Convert the list
		unstructured := ingressRoute.UnstructuredContent()
		for _, v := range unstructured {
			// Use regex to get the DNS names
			temp := fmt.Sprint(v)
			re := regexp.MustCompile("`(.*?)`")
			match := re.FindStringSubmatch(temp)
			if len(match) > 0 {
				// remove first and last characters
				match[0] = match[0][1 : len(match[0])-1]
				// resolve dns
				ip, err := resolveDNS(match[0])
				if err != nil {
					continue
				}
				// save DNS name and IP
				outList[match[0]] = ip
			}
		}
	}
}

func resolveDNS(name string) (string, error) {
	ips, err := net.LookupIP(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get IPs: %v\n", err)
		return "", err
	}
	return ips[0].String(), err
}

func createCsv(fName string) *os.File {
	csvFile, err := os.Create(fName)
	if err != nil {
		fmt.Printf("Failed to create csv file: %s", err)
		os.Exit(1)
	}
	return csvFile
}

func writeToCsv(c *os.File, hostNames map[string]string) {
	csvwriter := csv.NewWriter(c)
	// Write first row
	firstRow := make([]string, 0)
	firstRow = append(firstRow, "name")
	firstRow = append(firstRow, "ip")
	err := csvwriter.Write(firstRow)
	if err != nil {
		fmt.Printf("Failed to write in csv file: %s", err)
		os.Exit(1)
	}

	// Write all hosts and ips
	for host, ip := range hostNames {
		r := make([]string, 0)
		r = append(r, host)
		r = append(r, ip)
		err := csvwriter.Write(r)
		if err != nil {
			fmt.Printf("Failed to write in csv file: %s", err)
			os.Exit(1)
		}
	}

	csvwriter.Flush()
	c.Close()
}
