/**
 * Copyright (c) 2020 CoCreate LLC
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/autoscaler"
	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/config"
	ms "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/metricsource"
	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/provisioner"
	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/util"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	klog "k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
)

var (
	// CfgFile is a path to the configuration file
	CfgFile    string
	mainConfig config.Config

	longDesc = `
	nodeautoscaler usually runs in cluster as a controller.
	It watches node resource in the cluster, while periodically
	retrieves metrics of node from a metric source, e.g. Kubernetes metrics API,
	and triggers adding/removing node via interfaces of cloud providers or
	any platforms provisiong node resource to Kubernetes cluster.
	`
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "nodeautoscaler",
	Short: "nodeautoscaler automatically scales up/down Kuberneters cluster size based on node status",
	Long:  longDesc,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		defer klog.Flush()
		logger := klogr.New().WithName("root-cmd")

		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel()
			logger.V(4).Info("exit after context is canceled")
		}()

		go func() {
			<-signalCh
			logger.Info("exit on receiving SIGTERM or Interrupt")
			cancel()
		}()

		ac, err := autoscaler.NewAutoScaler(ctx, mainConfig)
		if err != nil {
			logger.Error(err, "failed to create auto scaler")
			panic(err)
		}

		ac.Run()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {

	cobra.OnInitialize(initConfig)

	// must put here before set flags
	mainConfig = config.NewConfig()
	config.Default(&mainConfig)

	setKlogFlags()
	setConfigFlags()
}

// initConfig reads in config file and ENV variables if set.
// Do not use klogr here as klogr is not initialized yet
func initConfig() {
	if CfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(CfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".nodeautoscaler" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".nodeautoscaler")
		CfgFile = home + "/" + ".nodeautoscaler.yaml"
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("Using config file %s", viper.ConfigFileUsed())
	}
}

func setKlogFlags() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")

	rootCmd.PersistentFlags().SetNormalizeFunc(util.NormalizeFunc)
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	rootCmd.PersistentFlags().MarkHidden("version")
	rootCmd.PersistentFlags().MarkHidden("log-flush-frequency")
	rootCmd.PersistentFlags().MarkHidden("alsologtostderr")
	rootCmd.PersistentFlags().MarkHidden("log-backtrace-at")
	rootCmd.PersistentFlags().MarkHidden("log-dir")
	rootCmd.PersistentFlags().MarkHidden("logtostderr")
	rootCmd.PersistentFlags().MarkHidden("stderrthreshold")
	rootCmd.PersistentFlags().MarkHidden("vmodule")
}

func setConfigFlags() {
	rootCmd.PersistentFlags().StringVar(&CfgFile, "config", CfgFile, "Path to config file")
	rootCmd.PersistentFlags().StringVar(&mainConfig.KubeConfigFile, "kubeconfig", "", "Path to a kubeconfig file, use in-cluster config if empty")
	rootCmd.PersistentFlags().StringVar(&mainConfig.LeaseLockName, "lease-lock-name", mainConfig.LeaseLockName, "The lease lock resource name")
	rootCmd.PersistentFlags().StringVar(&mainConfig.LeaseLockNamespace, "lease-lock-namespace", mainConfig.LeaseLockNamespace, "The lease lock resource namespace")
	rootCmd.PersistentFlags().IntVar(&mainConfig.CacheResyncPeriod, "cache-resync-period", mainConfig.CacheResyncPeriod, "The period in seconds "+
		"in which the available node list should be considered in metrics calculation is updated")
	rootCmd.PersistentFlags().StringVar(&mainConfig.AutoScaleGroupConfig, "auto-scale-group-config", mainConfig.AutoScaleGroupConfig,
		"The path to a file that claims configurations of different auto scale groups")

	/*
	 * below parameters can be overrided by auto scale group specific configurations
	 *
	 */
	rootCmd.PersistentFlags().StringVar((*string)(&mainConfig.MetricSource), "metric-source",
		string(mainConfig.MetricSource), fmt.Sprintf("Type of metric source, support %s", ms.SupportSourceType()))
	rootCmd.PersistentFlags().StringVar(&mainConfig.LabelSelector, "label-selector", mainConfig.LabelSelector, "A list of \"label_name=label_value\" "+
		"separated by comma. Nodes with exactly matching label set are watched by autoscaler. All nodes are processed if set to empty")
	rootCmd.PersistentFlags().IntVar(&mainConfig.MetricsCalculatePeriod, "metrics-cal-period", mainConfig.MetricsCalculatePeriod,
		"The period in seconds in which node metrics are recalculated")
	rootCmd.PersistentFlags().StringVar(&mainConfig.ScaleUpThreshold, "scale-up-threshold", mainConfig.ScaleUpThreshold, "Thresholds, in ratio, exceeding which a scale up is triggered")
	rootCmd.PersistentFlags().StringVar(&mainConfig.ScaleDownThreshold, "scale-down-threshold", mainConfig.ScaleDownThreshold, "Thresholds, in ratio, below which a scale down is triggered")
	rootCmd.PersistentFlags().Int32Var(&mainConfig.AlarmWindow, "alarm-window", mainConfig.AlarmWindow, "How long in seconds before a scaling is triggred "+
		"since a break of a threshold is saw")
	rootCmd.PersistentFlags().Int32Var(&mainConfig.AlarmCoolDown, "alarm-cool-down", mainConfig.AlarmCoolDown, "Minimum cooling down time in seconds between 2 fired scaling. "+
		"Note that autoscaler always waiting for scaling really finishes in the backend, "+
		"so the actual waiting time between 2 scaling is backend_scaling_time + cool_down_time")
	rootCmd.PersistentFlags().Int32Var(&mainConfig.AlarmCancelWindow, "alarm-cancel-window", mainConfig.AlarmCancelWindow,
		"For how long in seconds metrics keep normal before an protential alarm could be canceled. This one should be larger than alarm-window")
	rootCmd.PersistentFlags().IntVar(&mainConfig.MetricCacheExpireTime, "metric-cache-expire-time", mainConfig.MetricCacheExpireTime,
		"For how long in seconds metrics can be read from cache since a update")
	rootCmd.PersistentFlags().Int32Var(&mainConfig.ScaleUpTimeout, "scale-up-timeout", mainConfig.ScaleUpTimeout,
		"After how long in seconds a scaling up time out")
	rootCmd.PersistentFlags().IntVar(&mainConfig.MaxBackendFailure, "max-backend-failure", mainConfig.MaxBackendFailure,
		"Maximum times of allowed provisioning failure in backend. "+"Only failures of scaling up are counted")
	rootCmd.PersistentFlags().StringVar((*string)(&mainConfig.BackendProvsioner), "backend-provisioner", string(mainConfig.BackendProvsioner),
		fmt.Sprintf("Type of backend provisioner provisioning nodes, support: %s", provisioner.SupportProvisionerType()))
	rootCmd.PersistentFlags().StringVar(&mainConfig.RancherURL, "rancher-url", mainConfig.RancherURL, "URL of Rancher to access if use Rancher as backend provisioner")
	rootCmd.PersistentFlags().StringVar(&mainConfig.RancherToken, "rancher-token", mainConfig.RancherToken,
		"Token used to access Rancher if use Rancher as backend provisioner")
	rootCmd.PersistentFlags().StringVar(&mainConfig.RancherAnnotationNamespace, "rancher-annotation-namespace", mainConfig.RancherAnnotationNamespace,
		"The name of the namespace with the annotation \""+provisioner.RancherProjAnnotation+"\" from which cluster ID is derived. "+
			"This means that autoscaler only lookups node pools in local cluster when \"ranchernodepool\" is used as the backend.")
	rootCmd.PersistentFlags().StringVar(&mainConfig.RancherNodePoolNamePrefix, "rancher-node-pool-name-prefix", mainConfig.RancherNodePoolNamePrefix,
		"The name prefix of the node pool in Rancher. Only nodes in this pool matching \"label-selector\" will be managed. "+
			"This only effects when \"ranchernodepool\" is used as the backend. This will be ignored if \"auto-scale-group-config\" is set"+
			"This means that autoscaler only lookups node pools in local cluster when ranchernodepool is used as the backend.")
	rootCmd.PersistentFlags().StringVar(&mainConfig.RancherCA, "rancher-ca", mainConfig.RancherCA, "Path to a CA to verify Rancher server, "+
		"insecure connection will be used if set to empty")
	rootCmd.PersistentFlags().IntVar(&mainConfig.MaxNodeNum, "max-node-num", mainConfig.MaxNodeNum,
		"Maximum number of nodes can exist after scaling up. Only blocks scaling up, i.e. no auto scaling down if nodes are redundant")
	rootCmd.PersistentFlags().IntVar(&mainConfig.MinNodeNum, "min-node-num", mainConfig.MinNodeNum,
		"Minimum number of existing nodes before scaling down. Only blocks scaling down, i.e. no auto scaling up if nodes are insufficient")

	// hide flags of unfinished functionalities
	rootCmd.PersistentFlags().MarkHidden("lease-lock-name")
	rootCmd.PersistentFlags().MarkHidden("lease-lock-namespace")

}
