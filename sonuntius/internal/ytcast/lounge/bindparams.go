// Maps to: src/lib/app/BindParams.ts
//
// BindParams holds the query-string parameters that ride along with every
// request to the lounge `/bind` endpoint. Upstream's class is mutable and
// load-bearing for the protocol: SID / gsessionid / AID / RID identify
// the session and serialize messages, and the comment block at the top of
// the upstream file explains the arithmetic:
//
//   - RID starts random in [41000, 49999] and increments per send.
//   - AID starts at 3 and tracks the highest AID seen on incoming frames.
//   - SID + gsessionid are returned by the receiver in the first `c` /
//     `S` frames and pinned for the rest of the session.
//
// The Go port preserves the same arithmetic byte-for-byte. Off-by-ones in
// this file silently desync the session — every change should be paired
// with a unit-test update.
package lounge

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strconv"

	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// DeviceInfo mirrors upstream's `interface DeviceInfo`. Wire-encoded as
// JSON inside the `deviceInfo` query parameter.
type DeviceInfo struct {
	Brand                          string `json:"brand"`
	Model                          string `json:"model"`
	Year                           int    `json:"year"`
	OS                             string `json:"os"`
	OSVersion                      string `json:"osVersion"`
	Chipset                        string `json:"chipset"`
	ClientName                     string `json:"clientName"`
	DialAdditionalDataSupportLevel string `json:"dialAdditionalDataSupportLevel"`
	MDXDialServerType              string `json:"mdxDialServerType"`
}

// defaultDeviceInfo mirrors upstream's `DEFAULT_DEVICE_INFO`.
func defaultDeviceInfo(brand, model string) DeviceInfo {
	return DeviceInfo{
		Brand:                          brand,
		Model:                          model,
		Year:                           0,
		OS:                             "Windows",
		OSVersion:                      "10.0",
		Chipset:                        "",
		ClientName:                     "TVHTML5",
		DialAdditionalDataSupportLevel: "unsupported",
		MDXDialServerType:              "MDX_DIAL_SERVER_TYPE_UNKNOWN",
	}
}

// BindParamsInitOptions mirrors upstream's `interface
// BindParamsInitOptions`.
type BindParamsInitOptions struct {
	Theme      string
	DeviceID   string
	ScreenName string
	ScreenApp  string
	Brand      string
	Model      string
}

// LoungeToken ports `interface LoungeToken` from Session.ts. Kept here
// because BindParams holds a reference to its `loungeToken` field.
type LoungeToken struct {
	Expiration               int64  `json:"expiration"`
	LoungeToken              string `json:"loungeToken"`
	LoungeTokenLifespanMs    int64  `json:"loungeTokenLifespanMs"`
	RefreshIntervalInMillis  int64  `json:"refreshIntervalInMillis"`
	ScreenID                 string `json:"screenId"`
}

// MDXContext ports `interface MDXContext` — the small struct persisted in
// the data store so that a restart can resume with the same device id /
// screen id.
type MDXContext struct {
	DeviceID string `json:"deviceId,omitempty"`
	ScreenID string `json:"screenId,omitempty"`
}

// BindParams ports the upstream `class BindParams`. Methods are
// non-thread-safe — Session owns the only writer and serializes access
// through its task queue (matching upstream).
type BindParams struct {
	Device          string
	ID              string // Device ID
	ObfuscatedGaiaID string
	Name             string // Screen name
	App              string // Screen app
	Theme            string
	Capabilities     string
	CST              string
	MDXVersion       int
	LoungeIDToken    string
	VER              int
	V                int
	DeviceInfo       DeviceInfo
	SID              string
	RID              int
	CVER             int
	AID              int
	GSessionID       string
	ZX               string
	T                int
}

// NewBindParams constructs a BindParams with the same defaults upstream's
// constructor applies. RID is randomized in [41000, 49999], ZX is a 12-
// char hex string, AID starts at 3.
func NewBindParams(opts BindParamsInitOptions) *BindParams {
	bp := &BindParams{
		Device:           "LOUNGE_SCREEN",
		ID:               opts.DeviceID,
		ObfuscatedGaiaID: "",
		Name:             opts.ScreenName,
		App:              opts.ScreenApp,
		Theme:            opts.Theme,
		Capabilities:     "dsp,mic,dpa,ntb",
		CST:              "m",
		MDXVersion:       2,
		VER:              8,
		V:                2,
		DeviceInfo:       defaultDeviceInfo(opts.Brand, opts.Model),
		CVER:             1,
		ZX:               generateZX(),
		T:                1,
	}
	bp.RID = generateRID()
	bp.AID = 3
	return bp
}

// Reset ports `reset()` — clears SID / loungeIdToken / gsessionid and
// re-randomizes RID, restoring AID to 3.
func (bp *BindParams) Reset() {
	bp.SID = ""
	bp.LoungeIDToken = ""
	bp.GSessionID = ""
	bp.RID = generateRID()
	bp.AID = 3
}

// UpdateWithMDXContext ports `updateWithMdxContext`. Only the deviceId
// field is honored — upstream behaviour exactly.
func (bp *BindParams) UpdateWithMDXContext(ctx MDXContext) {
	if ctx.DeviceID != "" {
		bp.ID = ctx.DeviceID
	}
}

// UpdateWithLoungeToken ports `updateWithLoungeToken`.
func (bp *BindParams) UpdateWithLoungeToken(token LoungeToken) {
	bp.LoungeIDToken = token.LoungeToken
}

// UpdateWithMessage ports `updateWithMessage`. Each message may pin SID
// (name == "c", first payload element) or gsessionid (name == "S",
// payload as string). AID is tracked monotonically — never decreases.
func (bp *BindParams) UpdateWithMessage(messages ...*Message) {
	for _, cmd := range messages {
		if cmd == nil {
			continue
		}
		switch cmd.Name {
		case "c":
			// Payload is the array tail; upstream reads payload[0].
			arr := cmd.PayloadAsArray()
			if len(arr) > 0 {
				if sid, ok := arr[0].(string); ok {
					bp.SID = sid
				}
			}
		case "S":
			// Payload is a bare string (unwrapped from the single-element array).
			if sid := cmd.PayloadAsString(); sid != "" {
				bp.GSessionID = sid
			}
		}
		if cmd.AID != nil && *cmd.AID > bp.AID {
			bp.AID = *cmd.AID
		}
	}
}

// QueryStringType enumerates the three ways upstream serializes
// BindParams into a query string.
type QueryStringType string

const (
	// QueryStringTypeInitSession produces the params for the first POST
	// to /bind that establishes SID / gsessionid.
	QueryStringTypeInitSession QueryStringType = "initSession"
	// QueryStringTypeSendMessage produces the params for an outbound
	// message POST.
	QueryStringTypeSendMessage QueryStringType = "sendMessage"
	// QueryStringTypeRPC produces the params for the long-poll GET that
	// streams incoming messages.
	QueryStringTypeRPC QueryStringType = "rpc"
)

// ToQueryString ports `toQueryString(type, AID)`. Returns the encoded
// query string and may mutate AID / RID per the rules below.
//
// - initSession: bumps RID++.
// - sendMessage: AID is the max of (existing AID, supplied AID) if both
//                are present; bumps RID++; bumps AID++ when no AID was
//                supplied by the caller.
// - rpc:        no mutation.
//
// Returns *IncompleteAPIDataError when required fields are missing.
//
// The `aid` parameter mirrors the optional `AID?: number | null`
// upstream takes — pass nil for the rpc and initSession cases.
func (bp *BindParams) ToQueryString(kind QueryStringType, aid *int) (string, error) {
	missing := make([]string, 0)
	if bp.LoungeIDToken == "" {
		missing = append(missing, "loungeIdToken")
	}
	if kind == QueryStringTypeSendMessage || kind == QueryStringTypeRPC {
		if bp.SID == "" {
			missing = append(missing, "SID")
		}
		if bp.GSessionID == "" {
			missing = append(missing, "gsessionid")
		}
	}
	if len(missing) > 0 {
		return "", yterrors.NewIncompleteAPIDataError(
			"Missing data required to construct query string from bind params",
			missing,
		)
	}

	// Common params shared between all three types.
	v := url.Values{}
	v.Set("device", bp.Device)
	v.Set("id", bp.ID)
	v.Set("obfuscatedGaiaId", bp.ObfuscatedGaiaID)
	v.Set("name", bp.Name)
	v.Set("app", bp.App)
	v.Set("theme", bp.Theme)
	v.Set("capabilities", bp.Capabilities)
	v.Set("cst", bp.CST)
	v.Set("mdxVersion", strconv.Itoa(bp.MDXVersion))
	v.Set("loungeIdToken", bp.LoungeIDToken)
	v.Set("VER", strconv.Itoa(bp.VER))
	v.Set("v", strconv.Itoa(bp.V))
	v.Set("zx", generateZX())
	v.Set("t", strconv.Itoa(bp.T))

	switch kind {
	case QueryStringTypeInitSession:
		deviceInfo, err := json.Marshal(bp.DeviceInfo)
		if err != nil {
			return "", fmt.Errorf("marshal deviceInfo: %w", err)
		}
		v.Set("deviceInfo", string(deviceInfo))
		v.Set("RID", strconv.Itoa(bp.RID))
		v.Set("CVER", strconv.Itoa(bp.CVER))
		bp.RID++

	case QueryStringTypeSendMessage:
		// AID arithmetic mirrors upstream verbatim:
		//   this.AID = AID && this.AID ? Math.max(AID, this.AID) : AID || this.AID;
		// JS-truthy semantics: 0 counts as falsy.
		if aid != nil && *aid != 0 && bp.AID != 0 {
			if *aid > bp.AID {
				bp.AID = *aid
			}
		} else if aid != nil && *aid != 0 {
			bp.AID = *aid
		}
		// else: aid is nil or zero — keep bp.AID as-is.

		deviceInfo, err := json.Marshal(bp.DeviceInfo)
		if err != nil {
			return "", fmt.Errorf("marshal deviceInfo: %w", err)
		}
		v.Set("deviceInfo", string(deviceInfo))
		v.Set("SID", bp.SID)
		v.Set("RID", strconv.Itoa(bp.RID))
		v.Set("AID", strconv.Itoa(bp.AID))
		v.Set("gsessionid", bp.GSessionID)
		// Upstream's `if (!AID) { this.AID++; }` — uses JS truthiness so
		// nil OR zero increments.
		if aid == nil || *aid == 0 {
			bp.AID++
		}
		bp.RID++

	case QueryStringTypeRPC:
		v.Set("RID", "rpc")
		v.Set("SID", bp.SID)
		v.Set("CI", "0")
		v.Set("AID", strconv.Itoa(bp.AID))
		v.Set("gsessionid", bp.GSessionID)
		v.Set("TYPE", "xmlhttp")

	default:
		return "", fmt.Errorf("unknown QueryStringType: %q", kind)
	}

	return v.Encode(), nil
}

// generateZX ports the upstream `#generateZX` — uuidv4 with dashes
// stripped, truncated to 12 hex chars. We use crypto/rand directly
// (Go has no stdlib uuid v4 generator) and emit 12 hex chars from 6
// random bytes.
func generateZX() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failure is exceedingly rare and not recoverable
		// for our purposes — emit a deterministic fallback rather than
		// panicking so the lounge keeps trying.
		return "000000000000"
	}
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
}

// generateRID ports the upstream `#generateRID` — a uniform random
// integer in [41000, 49999].
func generateRID() int {
	const (
		min = 41000
		max = 49999
	)
	span := big.NewInt(int64(max - min + 1))
	n, err := rand.Int(rand.Reader, span)
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

// generateDeviceID is a Go-only helper mirroring upstream's `uuidv4()`
// call inside the Session constructor for the device id. We emit a
// hex-encoded 16-byte value with version bits set per RFC 4122 §4.4.
func generateDeviceID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	// Set version (4) and variant (RFC 4122) bits.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		buf[0], buf[1], buf[2], buf[3],
		buf[4], buf[5],
		buf[6], buf[7],
		buf[8], buf[9],
		buf[10], buf[11], buf[12], buf[13], buf[14], buf[15])
}
