import { useState } from "react";
import { DevicesTab } from "./DevicesTab";
import { NetworkTab } from "./NetworkTab";
import { TunnelTab } from "./TunnelTab";

type SettingsTab = "devices" | "network" | "tunnel";

export function SettingsView({ onClose, onError }: { onClose: () => void; onError: (message: string) => void }) {
  const [tab, setTab] = useState<SettingsTab>("devices");

  return (
    <div className="settings-view">
      <header className="settings-header">
        <div>
          <h2>Settings</h2>
          <p>Devices, network access, and the remote tunnel.</p>
        </div>
        <button type="button" className="icon-button" onClick={onClose} aria-label="Close settings and return to sessions">
          <span aria-hidden="true">✕</span>
        </button>
      </header>

      <div className="segmented-control settings-tabs" role="tablist" aria-label="Settings sections">
        <button
          type="button"
          role="tab"
          aria-selected={tab === "devices"}
          className={tab === "devices" ? "is-selected" : ""}
          onClick={() => setTab("devices")}
        >
          Devices
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "network"}
          className={tab === "network" ? "is-selected" : ""}
          onClick={() => setTab("network")}
        >
          Network
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "tunnel"}
          className={tab === "tunnel" ? "is-selected" : ""}
          onClick={() => setTab("tunnel")}
        >
          Tunnel
        </button>
      </div>

      <div className="settings-body" role="tabpanel" aria-label={tab}>
        {tab === "devices" && <DevicesTab onError={onError} />}
        {tab === "network" && <NetworkTab onError={onError} />}
        {tab === "tunnel" && <TunnelTab onError={onError} />}
      </div>
    </div>
  );
}
