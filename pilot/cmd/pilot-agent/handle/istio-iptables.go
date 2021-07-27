package handle

import (
	"encoding/json"
	"github.com/imdario/mergo"
	"github.com/spf13/viper"
	"io/ioutil"
	"istio.io/istio/tools/istio-iptables/pkg/cmd"
	"istio.io/istio/tools/istio-iptables/pkg/config"
	"istio.io/istio/tools/istio-iptables/pkg/constants"
	dep "istio.io/istio/tools/istio-iptables/pkg/dependencies"
	"istio.io/istio/tools/istio-iptables/pkg/validation"
	"istio.io/pkg/log"
	"net/http"
	"strconv"
	"strings"
)

var cfg *config.Config

func IstioIptables(w http.ResponseWriter, r *http.Request) {
	log.Info("set iptables")
	cfg = cmd.ConstructConfig()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c := &config.Config{}
	err = json.Unmarshal(body, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = mergo.Merge(cfg, c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Infof("config:---> %+v", cfg)

	var ext dep.Dependencies
	if cfg.DryRun {
		ext = &dep.StdoutStubDependencies{}
	} else {
		ext = &dep.RealDependencies{}
	}

	iptConfigurator := cmd.NewIptablesConfigurator(cfg, ext)
	if !cfg.SkipRuleApply {
		iptConfigurator.Run()
	}
	if cfg.RunValidation {
		hostIP, err := cmd.GetLocalIP()
		if err != nil {
			// Assume it is not handled by istio-cni and won't reuse the ValidationErrorCode
			http.Error(w, err.Error(), http.StatusInternalServerError)
			// panic(err)
		}
		validator := validation.NewValidator(cfg, hostIP)

		err = validator.Run()
		if err != nil {
			http.Error(w, err.Error(), constants.ValidationErrorCode)

		}
	}
}

func init() {
	// Read in all environment variables
	viper.AutomaticEnv()
	// Replace - with _; so that environment variables are looked up correctly.
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	var envoyPort = "15001"
	var inboundPort = "15006"

	viper.SetDefault(constants.EnvoyPort, envoyPort)

	viper.SetDefault(constants.InboundCapturePort, inboundPort)

	viper.SetDefault(constants.ProxyUID, "")

	viper.SetDefault(constants.ProxyGID, "")

	viper.SetDefault(constants.InboundInterceptionMode, "")

	viper.SetDefault(constants.InboundPorts, "")

	viper.SetDefault(constants.LocalExcludePorts, "")

	viper.SetDefault(constants.ServiceCidr, "")

	viper.SetDefault(constants.ServiceExcludeCidr, "")

	viper.SetDefault(constants.LocalOutboundPortsExclude, "")

	viper.SetDefault(constants.KubeVirtInterfaces, "")

	viper.SetDefault(constants.InboundTProxyMark, "1337")

	viper.SetDefault(constants.InboundTProxyRouteTable, "133")

	viper.SetDefault(constants.DryRun, false)

	viper.SetDefault(constants.RestoreFormat, true)

	viper.SetDefault(constants.IptablesProbePort, strconv.Itoa(constants.DefaultIptablesProbePort))

	viper.SetDefault(constants.ProbeTimeout, constants.DefaultProbeTimeout)

	viper.SetDefault(constants.SkipRuleApply, false)

	viper.SetDefault(constants.RunValidation, false)

}
