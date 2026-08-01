import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { PairedDevice, PairingRequest } from "../types";

export function PairingPanel() {
  const [requests, setRequests] = useState<PairingRequest[]>([]);
  const [devices, setDevices] = useState<PairedDevice[]>([]);
  const [expanded, setExpanded] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [reqs, devs] = await Promise.all([
        api.pairingRequests(),
        api.pairedDevices(),
      ]);
      setRequests(reqs);
      setDevices(devs);
    } catch {
      // Pairing endpoints may not be available
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  const hasActivity = requests.length > 0 || devices.length > 0;

  if (!hasActivity && !expanded) {
    return (
      <button
        type="button"
        className="pairing-toggle"
        onClick={() => { setExpanded(true); refresh(); }}
        title="Device pairing"
      >
        <span aria-hidden="true">🔗</span> Devices
      </button>
    );
  }

  return (
    <div className="pairing-panel">
      <button
        type="button"
        className="pairing-toggle"
        onClick={() => setExpanded(!expanded)}
      >
        <span aria-hidden="true">🔗</span> Devices
        {requests.length > 0 && <span className="pairing-badge">{requests.length}</span>}
      </button>

      {expanded && (
        <div className="pairing-content">
          {requests.length > 0 && (
            <section className="pairing-section">
              <h3>Pairing Requests</h3>
              {requests.map((req) => (
                <PairingRequestCard
                  key={req.device_id}
                  request={req}
                  onAccept={async () => {
                    await api.acceptPairing(req.device_id);
                    refresh();
                  }}
                  onReject={async () => {
                    await api.rejectPairing(req.device_id);
                    refresh();
                  }}
                />
              ))}
            </section>
          )}

          {devices.length > 0 && (
            <section className="pairing-section">
              <h3>Paired Devices</h3>
              {devices.map((dev) => (
                <PairedDeviceCard
                  key={dev.device_id}
                  device={dev}
                  onRemove={async () => {
                    await api.removePairedDevice(dev.device_id);
                    refresh();
                  }}
                />
              ))}
            </section>
          )}

          {requests.length === 0 && devices.length === 0 && (
            <p className="pairing-empty">No paired devices. Devices will appear here when they request pairing.</p>
          )}
        </div>
      )}
    </div>
  );
}

function PairingRequestCard({
  request,
  onAccept,
  onReject,
}: {
  request: PairingRequest;
  onAccept: () => void;
  onReject: () => void;
}) {
  return (
    <div className="pairing-card pairing-request">
      <div className="pairing-card-info">
        <strong>{request.device_name}</strong>
        <span className="pairing-meta">
          {request.platform} · {request.device_id.slice(0, 8)}
        </span>
      </div>
      <div className="pairing-card-actions">
        <button type="button" className="pairing-accept" onClick={onAccept}>Accept</button>
        <button type="button" className="pairing-reject" onClick={onReject}>Reject</button>
      </div>
    </div>
  );
}

function PairedDeviceCard({
  device,
  onRemove,
}: {
  device: PairedDevice;
  onRemove: () => void;
}) {
  return (
    <div className="pairing-card pairing-device">
      <div className="pairing-card-info">
        <strong>{device.device_name}</strong>
        <span className="pairing-meta">
          {device.platform} · {device.device_id.slice(0, 8)}
        </span>
        <span className="pairing-time">
          Last seen: {new Date(device.last_seen).toLocaleString()}
        </span>
      </div>
      <div className="pairing-card-actions">
        <button type="button" className="pairing-remove" onClick={onRemove}>Remove</button>
      </div>
    </div>
  );
}
