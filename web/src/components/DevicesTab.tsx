import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { PairedDevice, PairingRequest } from "../types";

export function DevicesTab({ onError }: { onError: (message: string) => void }) {
  const [requests, setRequests] = useState<PairingRequest[]>([]);
  const [devices, setDevices] = useState<PairedDevice[]>([]);

  const refresh = useCallback(async () => {
    try {
      const [nextRequests, nextDevices] = await Promise.all([
        api.pairingRequests(),
        api.pairedDevices()
      ]);
      setRequests(nextRequests);
      setDevices(nextDevices);
    } catch (err) {
      onError((err as Error).message);
    }
  }, [onError]);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  const accept = async (deviceID: string) => {
    try {
      await api.acceptPairing(deviceID);
      await refresh();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  const reject = async (deviceID: string) => {
    try {
      await api.rejectPairing(deviceID);
      await refresh();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  return (
    <div className="settings-sections">
      <section className="settings-section" aria-labelledby="pairing-requests-heading">
        <h3 id="pairing-requests-heading">Connection requests</h3>
        <p className="settings-hint">
          Approve only when the 6-digit code shown here matches the code on the requesting device.
        </p>
        {requests.length === 0 ? (
          <p className="settings-empty">No pending requests.</p>
        ) : (
          <ul className="request-list">
            {requests.map((request) => (
              <li key={request.device_id} className="request-card">
                <div className="request-info">
                  <strong>{request.device_name}</strong>
                  <span className="request-meta">
                    {request.platform || "unknown"} · {request.type === "web" ? "web browser" : "mobile app"}
                  </span>
                </div>
                <div className="request-code" aria-label={`Verification code ${request.code || ""}`}>
                  {(request.code || "").split("").map((digit, index) => (
                    <span key={index} className="request-code-digit">{digit}</span>
                  ))}
                </div>
                <div className="request-actions">
                  <button type="button" className="primary-button" onClick={() => accept(request.device_id)}>
                    Accept
                  </button>
                  <button type="button" className="danger-button" onClick={() => reject(request.device_id)}>
                    Reject
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="settings-section" aria-labelledby="paired-devices-heading">
        <h3 id="paired-devices-heading">Connected devices</h3>
        {devices.length === 0 ? (
          <p className="settings-empty">No paired devices yet. Devices you approve will appear here.</p>
        ) : (
          <ul className="device-list">
            {devices.map((device) => (
              <DeviceRow key={device.device_id} device={device} onChanged={refresh} onError={onError} />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function DeviceRow({
  device,
  onChanged,
  onError
}: {
  device: PairedDevice;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(device.custom_name || "");
  const isWeb = device.type === "web";
  const displayName = device.custom_name || device.device_name;
  const lastSeen = device.last_seen ? new Date(device.last_seen).toLocaleString() : "unknown";

  const saveName = async () => {
    try {
      await api.renamePairedDevice(device.device_id, name.trim());
      setRenaming(false);
      onChanged();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  const resetName = async () => {
    try {
      await api.renamePairedDevice(device.device_id, "");
      setName("");
      setRenaming(false);
      onChanged();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  const remove = async () => {
    try {
      await api.removePairedDevice(device.device_id);
      onChanged();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  return (
    <li className="device-row">
      <span className={isWeb ? "device-type-icon web" : "device-type-icon"} aria-hidden="true">
        {isWeb ? "🖥" : "📱"}
      </span>
      {renaming ? (
        <div className="device-rename">
          <label>
            <span className="visually-hidden">Device name</span>
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
              placeholder={device.device_name}
              onKeyDown={(event) => {
                if (event.key === "Enter") saveName();
                if (event.key === "Escape") setRenaming(false);
              }}
            />
          </label>
          <button type="button" className="primary-button" onClick={saveName}>Save</button>
          <button type="button" className="quiet-button" onClick={() => setRenaming(false)}>Cancel</button>
        </div>
      ) : (
        <div className="device-info">
          <strong>{displayName}</strong>
          <span className="device-meta">
            {isWeb ? "web" : device.platform || "mobile"} · last seen {lastSeen}
          </span>
        </div>
      )}
      <div className="device-actions">
        <button type="button" className="quiet-button" onClick={() => { setName(device.custom_name || ""); setRenaming(!renaming); }}>
          {device.custom_name ? "Rename" : "Name"}
        </button>
        {device.custom_name && !renaming && (
          <button type="button" className="quiet-button" onClick={resetName}>
            Reset
          </button>
        )}
        <button type="button" className="danger-button" onClick={remove}>Remove</button>
      </div>
    </li>
  );
}
