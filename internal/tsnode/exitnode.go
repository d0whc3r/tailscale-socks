package tsnode

import (
	"fmt"
	"strings"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

// setExitNode fills the exit-node fields of mp from a user-supplied argument:
//
//	"", "off", "none"   no exit node
//	"auto"              let Tailscale pick the best one (same as "auto:any")
//	"auto:<expr>"       an expression such as "auto:any" or "auto:geo:us"
//	<peer-name> | <ip>  a specific exit node
//
// st is the current status; it is used to resolve peer names.
func setExitNode(mp *ipn.MaskedPrefs, arg string, st *ipnstate.Status) error {
	mp.ExitNodeIDSet = true
	mp.ExitNodeIPSet = true
	mp.AutoExitNodeSet = true

	switch arg = strings.TrimSpace(arg); strings.ToLower(arg) {
	case "", "off", "none":
		mp.Prefs.ClearExitNode()
		return nil
	case "auto":
		mp.Prefs.AutoExitNode = "any"
		return nil
	}
	if expr, ok := ipn.ParseAutoExitNodeString(arg); ok {
		mp.Prefs.AutoExitNode = expr
		return nil
	}
	if err := mp.Prefs.SetExitNodeIP(arg, st); err != nil {
		return fmt.Errorf("exit node %q: %w", arg, err)
	}
	return nil
}
