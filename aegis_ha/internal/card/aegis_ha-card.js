/* AegisHA Alarm Card — a generic alarm_control_panel Lovelace card.
 *
 * Deliberately NOT a fork of alarmo-card: it talks only to the stock
 * alarm_control_panel services (alarm_arm_*/alarm_disarm/alarm_trigger)
 * and reads stock entity state + attributes, so it works against the
 * MQTT-discovered alarm_control_panel.aegis_ha entity with no custom
 * WebSocket commands. Authorization (per-user PINs) is entirely
 * server-side in AegisHA; the card only forwards the typed code.
 *
 * Usage (Lovelace):
 *   type: custom:aegis_ha-card
 *   entity: alarm_control_panel.aegis_ha
 */
const FEATURES = {
  ARM_HOME: 1,
  ARM_AWAY: 2,
  ARM_NIGHT: 4,
  TRIGGER: 8,
  ARM_CUSTOM_BYPASS: 16,
  ARM_VACATION: 32,
};

class AegisHACard extends HTMLElement {
  setConfig(config) {
    this._entity = (config && config.entity) || "alarm_control_panel.aegis_ha";
    this._title = (config && config.name) || "AegisHA";
    this._code = "";
    this._root = this.attachShadow({ mode: "open" });
  }

  set hass(hass) {
    this._hass = hass;
    this._render();
  }

  getCardSize() {
    return 6;
  }

  _st() {
    return this._hass && this._hass.states[this._entity];
  }

  _call(service) {
    const st = this._st();
    if (!st) return;
    this._hass.callService("alarm_control_panel", service, {
      entity_id: this._entity,
      code: this._code,
    });
    this._code = "";
    this._render();
  }

  _key(d) {
    this._code += d;
    const f = this._root.getElementById("code");
    if (f) f.textContent = "•".repeat(this._code.length);
  }

  _render() {
    const st = this._st();
    if (!st) {
      this._root.innerHTML = `<ha-card><div style="padding:16px">Unknown entity: ${this._entity}</div></ha-card>`;
      return;
    }
    const state = st.state;
    const feat = st.attributes.supported_features || 0;
    const codeFormat = st.attributes.code_format; // "number" | "text" | null
    const armBtns = [];
    if (feat & FEATURES.ARM_AWAY) armBtns.push(["away", "alarm_arm_away", "Away"]);
    if (feat & FEATURES.ARM_HOME) armBtns.push(["home", "alarm_arm_home", "Home"]);
    if (feat & FEATURES.ARM_NIGHT) armBtns.push(["night", "alarm_arm_night", "Night"]);
    if (feat & FEATURES.ARM_VACATION) armBtns.push(["vacation", "alarm_arm_vacation", "Vacation"]);
    if (feat & FEATURES.ARM_CUSTOM_BYPASS) armBtns.push(["custom", "alarm_arm_custom_bypass", "Custom"]);

    const colors = {
      disarmed: "#16a34a",
      arming: "#ef6c00",
      pending: "#ef6c00",
      triggered: "#c62828",
    };
    const bg = colors[state] || (state.startsWith("armed_") ? "#b71c1c" : "#37474f");

    const pad = codeFormat
      ? `<div class="pad">${[1, 2, 3, 4, 5, 6, 7, 8, 9, "clr", 0, "del"]
          .map(
            (d) =>
              `<button data-k="${d}">${d === "clr" ? "C" : d === "del" ? "⌫" : d}</button>`
          )
          .join("")}</div><div class="code" id="code"></div>`
      : "";

    this._root.innerHTML = `
      <ha-card>
        <style>
          .hdr { background:${bg}; color:#fff; padding:18px; text-align:center; border-radius:12px 12px 0 0; }
          .hdr .s { font-size:1.5rem; font-weight:700; text-transform:uppercase; letter-spacing:.04em; }
          .hdr .m { opacity:.85; font-size:.85rem; }
          .body { padding:14px; }
          .code { text-align:center; letter-spacing:.4em; min-height:1.2rem; margin:6px 0; }
          .pad { display:grid; grid-template-columns:repeat(3,1fr); gap:8px; margin-bottom:12px; }
          .pad button { font-size:1.3rem; padding:12px; border-radius:10px; border:1px solid var(--divider-color,#ccc); background:var(--card-background-color,#fff); cursor:pointer; }
          .modes { display:grid; grid-template-columns:repeat(2,1fr); gap:8px; }
          .modes button { padding:12px; border-radius:10px; border:none; background:var(--primary-color,#16a34a); color:#fff; font-weight:600; cursor:pointer; }
          .modes button.disarm { grid-column:1 / -1; background:#455a64; }
        </style>
        <div class="hdr">
          <div class="s">${state.replace("armed_", "")}</div>
          ${st.attributes.armed_by ? `<div class="m">by ${st.attributes.armed_by}</div>` : ""}
          ${state === "arming" || state === "pending" ? `<div class="m cd" id="cd"></div>` : ""}
          ${st.attributes.open_sensor_count ? `<div class="m">${st.attributes.open_sensor_count} open sensor(s)</div>` : ""}
        </div>
        <div class="body">
          ${pad}
          <div class="modes">
            ${armBtns
              .map(([m, svc, label]) => `<button data-svc="${svc}">Arm ${label}</button>`)
              .join("")}
            <button class="disarm" data-svc="alarm_disarm">Disarm</button>
          </div>
        </div>
      </ha-card>`;

    this._root.querySelectorAll(".pad button").forEach((b) => {
      b.onclick = () => {
        const k = b.getAttribute("data-k");
        if (k === "clr") {
          this._code = "";
          this._root.getElementById("code").textContent = "";
        } else if (k === "del") {
          this._code = this._code.slice(0, -1);
          this._root.getElementById("code").textContent = "•".repeat(this._code.length);
        } else {
          this._key(k);
        }
      };
    });
    this._root.querySelectorAll(".modes button").forEach((b) => {
      b.onclick = () => this._call(b.getAttribute("data-svc"));
    });

    // Live countdown for the exit (arming) / entry (pending) delay, driven
    // by the panel's delay_ends attribute (absolute unix end-time) so it
    // ticks client-side without per-second MQTT updates.
    this._clearTimer();
    if ((state === "arming" || state === "pending") && st.attributes.delay_ends) {
      this._startCountdown(Number(st.attributes.delay_ends));
    }
  }

  _startCountdown(endsUnix) {
    const tick = () => {
      const el = this._root.getElementById("cd");
      if (!el) {
        this._clearTimer();
        return;
      }
      const remaining = Math.max(0, Math.round(endsUnix - Date.now() / 1000));
      el.textContent = remaining + "s remaining";
      if (remaining <= 0) this._clearTimer();
    };
    tick();
    this._timer = setInterval(tick, 1000);
  }

  _clearTimer() {
    if (this._timer) {
      clearInterval(this._timer);
      this._timer = null;
    }
  }

  disconnectedCallback() {
    this._clearTimer();
  }
}

customElements.define("aegis_ha-card", AegisHACard);
window.customCards = window.customCards || [];
window.customCards.push({
  type: "aegis_ha-card",
  name: "AegisHA Alarm Card",
  description: "Keypad card for the AegisHA alarm_control_panel entity.",
  preview: true,
});
