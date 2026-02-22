package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(k8sCmd)
	k8sCmd.AddCommand(k8sPodsCmd)
	k8sCmd.AddCommand(k8sLogsCmd)
	k8sCmd.AddCommand(k8sEventsCmd)
	k8sCmd.AddCommand(k8sExecCmd)
	k8sCmd.AddCommand(k8sInfoCmd)
	k8sCmd.AddCommand(k8sTopCmd)
	k8sCmd.AddCommand(k8sK9sCmd)
}

var k8sCmd = &cobra.Command{
	Use:     "k8s",
	Aliases: []string{"kube"},
	Short:   "Kubernetes tools",
	Long:    "Kubernetes utilities wrapping kubectl and k9s.",
}

var k8sPodsCmd = &cobra.Command{
	Use:   "pods",
	Short: "List pods",
	RunE: func(cmd *cobra.Command, args []string) error {
		kubectlArgs := []string{"get", "pods"}
		ns, _ := cmd.Flags().GetString("namespace")
		allNs, _ := cmd.Flags().GetBool("all-namespaces")
		selector, _ := cmd.Flags().GetString("selector")

		if allNs {
			kubectlArgs = append(kubectlArgs, "--all-namespaces")
		} else if ns != "" {
			kubectlArgs = append(kubectlArgs, "-n", ns)
		}
		if selector != "" {
			kubectlArgs = append(kubectlArgs, "-l", selector)
		}
		kubectlArgs = append(kubectlArgs, "-o", "wide")

		return runKubectl(cmd, kubectlArgs)
	},
}

var k8sLogsCmd = &cobra.Command{
	Use:   "logs <pod>",
	Short: "Stream pod logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kubectlArgs := []string{"logs", args[0]}
		ns, _ := cmd.Flags().GetString("namespace")
		container, _ := cmd.Flags().GetString("container")
		follow, _ := cmd.Flags().GetBool("follow")
		tail, _ := cmd.Flags().GetInt("tail")
		previous, _ := cmd.Flags().GetBool("previous")

		if ns != "" {
			kubectlArgs = append(kubectlArgs, "-n", ns)
		}
		if container != "" {
			kubectlArgs = append(kubectlArgs, "-c", container)
		}
		if follow {
			kubectlArgs = append(kubectlArgs, "-f")
		}
		if tail > 0 {
			kubectlArgs = append(kubectlArgs, "--tail", fmt.Sprintf("%d", tail))
		}
		if previous {
			kubectlArgs = append(kubectlArgs, "--previous")
		}

		return runKubectl(cmd, kubectlArgs)
	},
}

var k8sEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show cluster events",
	RunE: func(cmd *cobra.Command, args []string) error {
		kubectlArgs := []string{"get", "events", "--sort-by=.lastTimestamp"}
		ns, _ := cmd.Flags().GetString("namespace")
		allNs, _ := cmd.Flags().GetBool("all-namespaces")

		if allNs {
			kubectlArgs = append(kubectlArgs, "--all-namespaces")
		} else if ns != "" {
			kubectlArgs = append(kubectlArgs, "-n", ns)
		}

		return runKubectl(cmd, kubectlArgs)
	},
}

var k8sExecCmd = &cobra.Command{
	Use:   "exec <pod> -- <command...>",
	Short: "Exec into a pod",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kubectlArgs := []string{"exec", "-it", args[0]}
		ns, _ := cmd.Flags().GetString("namespace")
		container, _ := cmd.Flags().GetString("container")

		if ns != "" {
			kubectlArgs = append(kubectlArgs, "-n", ns)
		}
		if container != "" {
			kubectlArgs = append(kubectlArgs, "-c", container)
		}

		// Everything after "--" is the command.
		kubectlArgs = append(kubectlArgs, "--")
		if len(args) > 1 {
			kubectlArgs = append(kubectlArgs, args[1:]...)
		} else {
			kubectlArgs = append(kubectlArgs, "/bin/sh")
		}

		return runKubectl(cmd, kubectlArgs)
	},
}

var k8sInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Cluster info summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKubectl(cmd, []string{"cluster-info"})
	},
}

var k8sTopCmd = &cobra.Command{
	Use:   "top",
	Short: "Show resource usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		pods, _ := cmd.Flags().GetBool("pods")
		nodes, _ := cmd.Flags().GetBool("nodes")
		ns, _ := cmd.Flags().GetString("namespace")

		if nodes {
			return runKubectl(cmd, []string{"top", "nodes"})
		}

		kubectlArgs := []string{"top", "pods"}
		if pods && ns != "" {
			kubectlArgs = append(kubectlArgs, "-n", ns)
		}
		return runKubectl(cmd, kubectlArgs)
	},
}

var k8sK9sCmd = &cobra.Command{
	Use:   "k9s",
	Short: "Launch k9s (if installed)",
	RunE: func(cmd *cobra.Command, args []string) error {
		k9sPath, err := exec.LookPath("k9s")
		if err != nil {
			fmt.Fprintln(os.Stderr, "k9s not found in PATH.")
			fmt.Fprintln(os.Stderr, "k9s is pre-installed in the sidequest container image.")
			return fmt.Errorf("k9s not found")
		}
		return syscall.Exec(k9sPath, append([]string{"k9s"}, args...), os.Environ())
	},
}

func init() {
	for _, cmd := range []*cobra.Command{k8sPodsCmd, k8sEventsCmd} {
		cmd.Flags().StringP("namespace", "n", "", "Kubernetes namespace")
		cmd.Flags().BoolP("all-namespaces", "A", false, "All namespaces")
		cmd.Flags().StringP("selector", "l", "", "Label selector")
	}

	k8sLogsCmd.Flags().StringP("namespace", "n", "", "Kubernetes namespace")
	k8sLogsCmd.Flags().StringP("container", "c", "", "Container name")
	k8sLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	k8sLogsCmd.Flags().Int("tail", 0, "Number of lines from the end")
	k8sLogsCmd.Flags().Bool("previous", false, "Show previous container logs")

	k8sExecCmd.Flags().StringP("namespace", "n", "", "Kubernetes namespace")
	k8sExecCmd.Flags().StringP("container", "c", "", "Container name")

	k8sTopCmd.Flags().Bool("pods", true, "Show pod resource usage")
	k8sTopCmd.Flags().Bool("nodes", false, "Show node resource usage")
	k8sTopCmd.Flags().StringP("namespace", "n", "", "Kubernetes namespace")
}

func runKubectl(cmd *cobra.Command, args []string) error {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		fmt.Fprintln(os.Stderr, "kubectl not found in PATH.")
		fmt.Fprintln(os.Stderr, "kubectl is pre-installed in the sidequest container image.")
		return fmt.Errorf("kubectl not found")
	}

	c := exec.Command(kubectlPath, args...)
	c.Stdin = os.Stdin
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.OutOrStderr()
	return c.Run()
}
