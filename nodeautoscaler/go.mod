module github.com/CoCreate-app/CoCreateLB/nodeautoscaler

go 1.15

replace k8s.io/client-go => k8s.io/client-go v0.18.8

require (
	github.com/go-logr/logr v0.4.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/rancher/norman v0.0.0-20210225010917-c7fd1e24145b
	github.com/rancher/types v0.0.0-20210123000350-7cb436b3f0b0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog/v2 v2.6.0
	k8s.io/metrics v0.18.8
)
