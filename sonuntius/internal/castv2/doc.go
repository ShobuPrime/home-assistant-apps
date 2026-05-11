// Maps to: N/A — package overview for the Cast (CASTV2) receiver layer.
//
// Package castv2 implements a Google Cast (CASTV2) receiver suitable for
// being discovered and driven by Cast senders such as the Android Tidal
// app. The receiver is metadata-only — it does not play audio; it parses
// the LOAD payloads delivered by senders and surfaces them as Intents for
// higher layers (Phase 3b's cmd binary) to map onto Music Assistant.
//
// Upstream references:
//
//   - Cast wire protocol (CastMessage proto + framing):
//     https://chromium.googlesource.com/openscreen/+/refs/heads/main/cast/common/channel/proto/cast_channel.proto
//   - AirReceiver-cert nonce-replay device-auth trick:
//     https://xakcop.com/post/shanocast/
//   - mDNS / DNS-SD wire format:
//     RFC 6762 (Multicast DNS) + RFC 6763 (DNS-Based Service Discovery)
//
// Package layout:
//
//   - castmessage.go     — hand-rolled CastMessage protobuf codec.
//   - framing.go         — 4-byte big-endian length-prefixed frame reader/writer.
//   - server.go          — TLS server: accepts senders on :8009, dispatches
//                          framed messages by namespace to handler functions.
//   - types.go           — public Go types shared across sub-packages.
//   - auth/airreceiver.go — AirReceiver responder: builds the AuthResponse
//                          protobuf from a user-supplied cert + signature.
//   - namespaces/        — per-namespace handlers (connection, heartbeat,
//                          receiver, media). The media handler defines a
//                          Parser interface; concrete parsers (Tidal,
//                          generic URL) are landed in Phase 3b.
//   - mdns/responder.go  — RFC 6762/6763 mDNS responder advertising the
//                          receiver under _googlecast._tcp.local.
//
// Phase 3a public-API surface (consumed by the Phase 3b cmd binary):
//
//	srv := castv2.NewServer(castv2.Options{
//	    Addr:          ":8009",
//	    TLSConfig:     tlsCfg, // built from cert/key on disk; nil disables the server
//	    AuthResponder: airReceiverResponder,
//	    Logger:        logger,
//	})
//	srv.RegisterParser(myParser)  // Phase 3b: Tidal + generic URL parsers
//	if err := srv.Start(ctx); err != nil { ... }
//	defer srv.Stop()
//
//	mdnsResp := mdns.NewResponder(mdns.Options{
//	    InstanceName: "Sonuntius (Tidal)",
//	    ServiceType:  "_googlecast._tcp",
//	    Port:         8009,
//	    UUID:         stableUUID,
//	    TXTRecords:   map[string]string{"md": "Chromecast", "ca": "4101", "ve": "05"},
//	    Logger:       logger,
//	})
//	if err := mdnsResp.Start(ctx); err != nil { ... }
//	defer mdnsResp.Stop()
//
// Stdlib-only — no third-party protobuf or mDNS dependencies.
package castv2
