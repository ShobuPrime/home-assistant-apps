# Cast device-auth certificates

The Cast proxy's TLS server presents an AirReceiver certificate so that
Cast senders accept the device-auth handshake. Per the app's privacy
posture we do **not** ship that certificate inside the container image.

The user supplies it under `/share/sonuntius/` on the HA host:

- `/share/sonuntius/airreceiver_cert.pem`
- `/share/sonuntius/airreceiver_key.pem`

See `DOCS.md` for provisioning instructions and provenance disclosure.
