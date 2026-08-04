// Maps to: src/lib/dial/DialServer.ts (launch body parsing inside `delegate.launchApp`)
//
// Upstream peer-dial hands the raw POST body straight through to
// `delegate.launchApp(appName, launchData, callback)`; the YouTubeApp
// callback then calls `parseLaunchData` (in src/lib/app/YouTubeApp.ts) to
// pull the pairing code out of the form-encoded body. We collapse those two
// steps here because the Server's OnLaunch contract surface already exposes
// the parsed PairingCode + leftover Sender map.
//
// The DIAL launch body is `application/x-www-form-urlencoded` for every
// real-world sender we have observed (Cast / YouTube TV App). YouTubeApp
// reads `pairingCode` and ignores everything else; we keep the rest under
// LaunchRequest.Sender so curious callers can inspect the spillover.
package dial

import (
	"net/url"
	"strings"
)

// pairingCodeKey is the form field name YouTube senders use. Lower-case in
// the upstream YouTubeApp.parseLaunchData call.
const pairingCodeKey = "pairingCode"

// parseLaunchPayload turns a raw DIAL launch POST body into a LaunchRequest.
// Empty and malformed bodies still yield a zero-value LaunchRequest with the
// raw bytes preserved — matching upstream's "best effort" behaviour where a
// missing pairing code does not abort the launch (the orchestrator surfaces
// a manual-pair UI instead).
func parseLaunchPayload(body []byte) LaunchRequest {
	out := LaunchRequest{
		Sender:  map[string]string{},
		RawBody: append([]byte(nil), body...),
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return out
	}
	values, err := url.ParseQuery(trimmed)
	if err != nil {
		// Upstream's parseLaunchData simply returns null on parse failure
		// — we do the same and let the OnLaunch callback decide what to
		// do with an empty PairingCode.
		return out
	}
	for k, vs := range values {
		joined := strings.Join(vs, ",")
		if k == pairingCodeKey {
			out.PairingCode = joined
			continue
		}
		out.Sender[k] = joined
	}
	return out
}
