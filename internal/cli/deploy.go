package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

const defaultImage = "registry.digitalocean.com/greenforests/sidequest:latest"
const defaultNamespace = "default"
const defaultName = "sidequest"

func init() {
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(undeployCmd)

	deployCmd.AddCommand(deployK8sCmd)
	undeployCmd.AddCommand(undeployK8sCmd)

	// deploy k8s flags
	deployK8sCmd.Flags().StringP("image", "i", defaultImage, "Container image")
	deployK8sCmd.Flags().StringP("namespace", "n", defaultNamespace, "Kubernetes namespace")
	deployK8sCmd.Flags().String("name", defaultName, "Resource name")
	deployK8sCmd.Flags().Int("replicas", 1, "Number of replicas")
	deployK8sCmd.Flags().Bool("port-forward", true, "Port-forward after deploy")
	deployK8sCmd.Flags().Int("local-port", 8080, "Local port for port-forward")
	deployK8sCmd.Flags().Int("target-port", 8080, "Target port for port-forward")

	// undeploy k8s flags
	undeployK8sCmd.Flags().StringP("namespace", "n", defaultNamespace, "Kubernetes namespace")
	undeployK8sCmd.Flags().String("name", defaultName, "Resource name")
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy sidequest to a target environment",
	Long:  "Deploy sidequest to Kubernetes or other environments.",
}

var undeployCmd = &cobra.Command{
	Use:   "undeploy",
	Short: "Remove sidequest from a target environment",
	Long:  "Remove sidequest from Kubernetes or other environments.",
}

// k8sManifestTmpl is the embedded Deployment + Service manifest template.
var k8sManifestTmpl = template.Must(template.New("k8s").Parse(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
  labels:
    app.kubernetes.io/name: {{ .Name }}
    app.kubernetes.io/managed-by: sidequest-cli
spec:
  replicas: {{ .Replicas }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ .Name }}
    spec:
      containers:
        - name: sidequest
          image: {{ .Image }}
          command: ["sidequest", "serve"]
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
            - name: rest
              containerPort: 8081
              protocol: TCP
            - name: grpc
              containerPort: 9090
              protocol: TCP
            - name: graphql
              containerPort: 8082
              protocol: TCP
          env:
            - name: SIDEQUEST_HTTP_ENABLED
              value: "true"
            - name: SIDEQUEST_REST_ENABLED
              value: "true"
            - name: SIDEQUEST_GRPC_ENABLED
              value: "true"
            - name: SIDEQUEST_GRAPHQL_ENABLED
              value: "true"
          readinessProbe:
            httpGet:
              path: /ready
              port: http
            initialDelaySeconds: 2
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
  labels:
    app.kubernetes.io/name: {{ .Name }}
    app.kubernetes.io/managed-by: sidequest-cli
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: {{ .Name }}
  ports:
    - name: http
      port: 8080
      targetPort: http
    - name: rest
      port: 8081
      targetPort: rest
    - name: grpc
      port: 9090
      targetPort: grpc
    - name: graphql
      port: 8082
      targetPort: graphql
`))

type k8sManifestData struct {
	Name      string
	Namespace string
	Image     string
	Replicas  int
}

var deployK8sCmd = &cobra.Command{
	Use:   "k8s",
	Short: "Deploy sidequest to Kubernetes",
	Long: `Deploy sidequest to a Kubernetes cluster.

Creates a Deployment and ClusterIP Service, waits for rollout to complete,
then optionally port-forwards to localhost. Cleans up on Ctrl+C.

Requires kubectl to be installed and configured.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		image, _ := cmd.Flags().GetString("image")
		namespace, _ := cmd.Flags().GetString("namespace")
		name, _ := cmd.Flags().GetString("name")
		replicas, _ := cmd.Flags().GetInt("replicas")
		portForward, _ := cmd.Flags().GetBool("port-forward")
		localPort, _ := cmd.Flags().GetInt("local-port")
		targetPort, _ := cmd.Flags().GetInt("target-port")

		// Verify kubectl is available.
		kubectlPath, err := exec.LookPath("kubectl")
		if err != nil {
			return fmt.Errorf("kubectl not found in PATH; required for k8s deployment")
		}

		// Render the manifest.
		data := k8sManifestData{
			Name:      name,
			Namespace: namespace,
			Image:     image,
			Replicas:  replicas,
		}
		var manifest strings.Builder
		if err := k8sManifestTmpl.Execute(&manifest, data); err != nil {
			return fmt.Errorf("rendering manifest: %w", err)
		}

		fmt.Printf("Deploying sidequest to namespace %q as %q\n", namespace, name)
		fmt.Printf("  Image:    %s\n", image)
		fmt.Printf("  Replicas: %d\n", replicas)
		fmt.Println()

		// Apply the manifest via kubectl apply -f -.
		applyCmd := exec.Command(kubectlPath, "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(manifest.String())
		applyCmd.Stdout = cmd.OutOrStdout()
		applyCmd.Stderr = cmd.OutOrStderr()
		if err := applyCmd.Run(); err != nil {
			return fmt.Errorf("kubectl apply failed: %w", err)
		}

		// Wait for rollout.
		fmt.Println("\nWaiting for rollout to complete...")
		rolloutCmd := exec.Command(kubectlPath, "rollout", "status",
			fmt.Sprintf("deployment/%s", name),
			"-n", namespace,
			"--timeout=120s",
		)
		rolloutCmd.Stdout = cmd.OutOrStdout()
		rolloutCmd.Stderr = cmd.OutOrStderr()
		if err := rolloutCmd.Run(); err != nil {
			return fmt.Errorf("rollout failed: %w", err)
		}

		fmt.Println("\nDeployment ready!")

		if !portForward {
			fmt.Println("\nDone. Use 'sidequest undeploy k8s' to remove.")
			return nil
		}

		// Port-forward with graceful cleanup on Ctrl+C.
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		pfArgs := []string{
			"port-forward",
			fmt.Sprintf("svc/%s", name),
			fmt.Sprintf("%d:%d", localPort, targetPort),
			"-n", namespace,
		}

		fmt.Printf("\nPort-forwarding localhost:%d -> svc/%s:%d\n", localPort, name, targetPort)
		fmt.Println("Press Ctrl+C to stop and clean up.")
		fmt.Println()

		pfCmd := exec.CommandContext(ctx, kubectlPath, pfArgs...)
		pfCmd.Stdout = cmd.OutOrStdout()
		pfCmd.Stderr = cmd.OutOrStderr()

		pfErr := pfCmd.Run()

		// Ctrl+C was pressed — clean up.
		fmt.Println("\nCleaning up resources...")
		if err := deleteK8sResources(kubectlPath, name, namespace, cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "Run 'sidequest undeploy k8s -n %s --name %s' to manually remove.\n", namespace, name)
		} else {
			fmt.Println("All resources removed.")
		}

		// Don't propagate context cancellation as an error.
		if ctx.Err() != nil {
			return nil
		}
		return pfErr
	},
}

var undeployK8sCmd = &cobra.Command{
	Use:   "k8s",
	Short: "Remove sidequest from Kubernetes",
	Long: `Remove the sidequest Deployment and Service from a Kubernetes cluster.

Requires kubectl to be installed and configured.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace, _ := cmd.Flags().GetString("namespace")
		name, _ := cmd.Flags().GetString("name")

		kubectlPath, err := exec.LookPath("kubectl")
		if err != nil {
			return fmt.Errorf("kubectl not found in PATH; required for k8s undeployment")
		}

		fmt.Printf("Removing sidequest %q from namespace %q...\n", name, namespace)

		if err := deleteK8sResources(kubectlPath, name, namespace, cmd); err != nil {
			return err
		}

		fmt.Println("All resources removed.")
		return nil
	},
}

// deleteK8sResources removes the Deployment and Service for the given name/namespace.
func deleteK8sResources(kubectlPath, name, namespace string, cmd *cobra.Command) error {
	// Delete both resources. Use --ignore-not-found so partial state is fine.
	for _, resource := range []string{"deployment", "service"} {
		delCmd := exec.Command(kubectlPath, "delete", resource, name,
			"-n", namespace,
			"--ignore-not-found",
			"--wait=true",
			fmt.Sprintf("--timeout=%s", (30*time.Second).String()),
		)
		delCmd.Stdout = cmd.OutOrStdout()
		delCmd.Stderr = cmd.OutOrStderr()
		if err := delCmd.Run(); err != nil {
			return fmt.Errorf("deleting %s/%s: %w", resource, name, err)
		}
	}
	return nil
}
