package handle

import (
	"istio.io/istio/tools/istio-clean-iptables/pkg/cmd"
	"istio.io/pkg/log"
	"net/http"
	"strconv"
)

func IstioCleanIptables(w http.ResponseWriter, r *http.Request) {
	log.Info("clean iptables")
	if r.Method != http.MethodGet {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	var dry bool
	val := r.URL.Query().Get("dry-run")
	dry, _ = strconv.ParseBool(val)
	cmd.Cleanup(dry)
}

type CleanIptables struct {
	DryRun bool `json:"dry_run"`
}
