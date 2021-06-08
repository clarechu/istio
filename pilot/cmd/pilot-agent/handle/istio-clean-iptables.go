package handle

import (
	"encoding/json"
	"io/ioutil"
	"istio.io/istio/tools/istio-clean-iptables/pkg/cmd"
	"istio.io/pkg/log"
	"net/http"
)

func IstioCleanIptables(w http.ResponseWriter, r *http.Request) {
	log.Info("clean iptables")
	if r.Method != http.MethodPost {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	clean := &CleanIptables{}
	err = json.Unmarshal(body, clean)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cmd.Cleanup(clean.DryRun)
}

type CleanIptables struct {
	DryRun bool `json:"dry_run"`
}
