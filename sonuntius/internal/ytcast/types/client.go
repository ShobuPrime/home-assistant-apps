// Maps to: src/lib/app/Client.ts
//
// Client identifies which YouTube surface a sender belongs to (regular YouTube
// or YouTube Music). Upstream couples Client with the `CLIENTS` table in
// Constants.ts — we co-locate the table here to keep the types package free of
// a constants → types → constants cycle.
package types

// ClientKey is the discriminant upstream uses for both YouTube and YouTube
// Music (the only two values today).
type ClientKey string

// Known ClientKey values. Strings match the wire / upstream values exactly.
const (
	ClientKeyYT      ClientKey = "YT"
	ClientKeyYTMusic ClientKey = "YTMUSIC"
)

// Client is the data record upstream defines as `interface Client { key,
// theme, name }`. Theme is the sender-side identifier used in protocol
// messages; Name is the human-readable label.
type Client struct {
	Key   ClientKey `json:"key"`
	Theme string    `json:"theme"`
	Name  string    `json:"name"`
}

// Clients enumerates every known client. Upstream stores the same record in
// `CLIENTS` inside Constants.ts; we keep the table next to the type that
// describes a row so the constants package can stay independent of types.
var Clients = map[ClientKey]Client{
	ClientKeyYT: {
		Key:   ClientKeyYT,
		Theme: "cl",
		Name:  "YouTube",
	},
	ClientKeyYTMusic: {
		Key:   ClientKeyYTMusic,
		Theme: "m",
		Name:  "YouTube Music",
	},
}

// ClientByTheme looks up a Client by its protocol `theme` value. Returns
// (zero, false) if no client matches — this is the lookup `Sender.parse` uses
// to populate the `client` field upstream.
func ClientByTheme(theme string) (Client, bool) {
	for _, c := range Clients {
		if c.Theme == theme {
			return c, true
		}
	}
	return Client{}, false
}
