package agent

import (
	"context"

	"github.com/spf13/cobra"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	hyperv1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/hypershift/cmd/nodepool/core"
)

type AgentPlatformCreateOptions struct{}

func NewAgentPlatformCreateOptions(_ *cobra.Command) *AgentPlatformCreateOptions {
	platformOpts := &AgentPlatformCreateOptions{}

	return platformOpts
}

func NewCreateCommand(coreOpts *core.CreateNodePoolOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "agent",
		Short:        "Creates basic functional NodePool resources for Agent platform",
		SilenceUsage: true,
	}

	platformOpts := NewAgentPlatformCreateOptions(cmd)

	cmd.RunE = coreOpts.CreateRunFunc(platformOpts)

	return cmd
}

func (o *AgentPlatformCreateOptions) UpdateNodePool(_ context.Context, _ *hyperv1.NodePool, _ *hyperv1.HostedCluster, _ crclient.Client) error {
	return nil
}

func (o *AgentPlatformCreateOptions) Type() hyperv1.PlatformType {
	return hyperv1.AgentPlatform
}

func (o *AgentPlatformCreateOptions) Validate() error {
	return nil
}
