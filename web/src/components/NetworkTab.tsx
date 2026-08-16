import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { NetworkClient, NetworkSettings } from "../types";

export function NetworkTab({ onError }: { onError: (message: string) => void }) {
  const [settings, setSettings] = useState<NetworkSettings | null>(null);
  const [clients, setClients] = useState<NetworkClient[]>([]);
  const [allowEntry, setAllowEntry] = useState("");
  const [banEntry, setBanEntry] = useState("");
  const [toggling, setToggling] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [nextSettings, nextClients] = await Promise.all([
        api.networkSettings(),
        api.networkClients()
      ]);
      setSettings(nextSettings);
      setClients(nextClients);
    } catch (err) {
      onError((err as Error).message);
    }
  }, [onError]);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  const toggleRemote = async (enabled: boolean) => {
    setToggling(true);
    try {
      setSettings(await api.updateNetworkSettings(enabled));
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setToggling(false);
    }
  };

  const addEntry = async (list: "allow" | "ban", entry: string, clear: () => void) => {
    try {
      setSettings(
        list === "allow"
          ? await api.addAllowEntry(entry.trim())
          : await api.addBanEntry(entry.trim())
      );
      clear();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  const removeEntry = async (list: "allow" | "ban", entry: string) => {
    try {
      setSettings(
        list === "allow"
          ? await api.removeAllowEntry(entry)
          : await api.removeBanEntry(entry)
      );
    } catch (err) {
      onError((err as Error).message);
    }
  };

  const submitAllow = (event: FormEvent) => {
    event.preventDefault();
    if (allowEntry.trim()) addEntry("allow", allowEntry, () => setAllowEntry(""));
  };

  const submitBan = (event: FormEvent) => {
    event.preventDefault();
    if (banEntry.trim()) addEntry("ban", banEntry, () => setBanEntry(""));
  };

  return (
    <div className="settings-sections">
      <section className="settings-section" aria-labelledby="network-status-heading">
        <h3 id="network-status-heading">LAN status</h3>
        <div className="network-summary">
          <div>
            <span className="settings-label">Daemon LAN IP</span>
            <p className="network-ip-list">{settings?.lan_ips?.length ? settings.lan_ips.join(", ") : "—"}</p>
          </div>
          <div className="toggle-row">
            <div>
              <span className="settings-label">Allow remote devices</span>
              <p className="settings-hint">
                When off, only this machine can reach the dashboard and API.
              </p>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={settings?.remote_access_enabled ?? true}
              className={settings?.remote_access_enabled ? "toggle-switch on" : "toggle-switch"}
              disabled={toggling || !settings}
              onClick={() => toggleRemote(!settings?.remote_access_enabled)}
            >
              <span className="visually-hidden">Allow remote devices to access the web interface</span>
              <span className="toggle-knob" aria-hidden="true" />
            </button>
          </div>
        </div>
      </section>

      <section className="settings-section" aria-labelledby="allowlist-heading">
        <h3 id="allowlist-heading">Allowed IPs</h3>
        <p className="settings-hint">Direct LAN connections are limited to these IPs and ranges. Empty allows the whole LAN.</p>
        <IPList entries={settings?.allowlist || []} onRemove={(entry) => removeEntry("allow", entry)} />
        <form className="ip-entry-form" onSubmit={submitAllow}>
          <label>
            <span className="visually-hidden">Add allowed IP or CIDR</span>
            <input
              value={allowEntry}
              onChange={(event) => setAllowEntry(event.target.value)}
              placeholder="192.168.1.0/24"
            />
          </label>
          <button type="submit" className="primary-button" disabled={!allowEntry.trim()}>Add</button>
        </form>
      </section>

      <section className="settings-section" aria-labelledby="banlist-heading">
        <h3 id="banlist-heading">Banned IPs</h3>
        <p className="settings-hint">Banned addresses are blocked on both LAN and tunnel connections.</p>
        <IPList entries={settings?.banlist || []} onRemove={(entry) => removeEntry("ban", entry)} />
        <form className="ip-entry-form" onSubmit={submitBan}>
          <label>
            <span className="visually-hidden">Add banned IP or CIDR</span>
            <input
              value={banEntry}
              onChange={(event) => setBanEntry(event.target.value)}
              placeholder="10.0.0.9"
            />
          </label>
          <button type="submit" className="danger-button" disabled={!banEntry.trim()}>Ban</button>
        </form>
      </section>

      <section className="settings-section" aria-labelledby="clients-heading">
        <h3 id="clients-heading">Connected devices</h3>
        {clients.length === 0 ? (
          <p className="settings-empty">No clients seen recently.</p>
        ) : (
          <table className="clients-table">
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">IP</th>
                <th scope="col">MAC</th>
                <th scope="col">Hostname</th>
                <th scope="col">Source</th>
                <th scope="col">Last seen</th>
                <th scope="col"><span className="visually-hidden">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {clients.map((client) => (
                <ClientRow key={client.key || client.ip} client={client} onChanged={refresh} onError={onError} />
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}

function IPList({ entries, onRemove }: { entries: string[]; onRemove: (entry: string) => void }) {
  if (entries.length === 0) {
    return <p className="settings-empty">Nothing listed.</p>;
  }
  return (
    <ul className="ip-chip-list">
      {entries.map((entry) => (
        <li key={entry} className="ip-chip">
          <code>{entry}</code>
          <button type="button" onClick={() => onRemove(entry)} aria-label={`Remove ${entry}`}>✕</button>
        </li>
      ))}
    </ul>
  );
}

function ClientRow({
  client,
  onChanged,
  onError
}: {
  client: NetworkClient;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(client.custom_name || "");
  const displayName = client.custom_name || client.hostname || "—";
  const lastSeen = client.last_seen ? new Date(client.last_seen * 1000).toLocaleTimeString() : "—";

  const save = async () => {
    try {
      await api.renameNetworkClient(client.key || client.ip, name.trim());
      setRenaming(false);
      onChanged();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  return (
    <tr>
      <td>
        {renaming ? (
          <div className="device-rename compact">
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
              placeholder="Custom name"
              onKeyDown={(event) => {
                if (event.key === "Enter") save();
                if (event.key === "Escape") setRenaming(false);
              }}
            />
            <button type="button" className="primary-button" onClick={save}>Save</button>
            <button type="button" className="quiet-button" onClick={() => setRenaming(false)}>Cancel</button>
          </div>
        ) : (
          <span className="client-name">{displayName}</span>
        )}
      </td>
      <td><code>{client.ip}</code></td>
      <td><code>{client.mac || "—"}</code></td>
      <td>{client.hostname || "—"}</td>
      <td>
        <span className={`client-class ${client.class}`}>{client.class}</span>
        {client.active_connections > 0 && (
          <span className="client-connections" title="Active connections"> · {client.active_connections} live</span>
        )}
      </td>
      <td>{lastSeen}</td>
      <td>
        <button type="button" className="quiet-button" onClick={() => { setName(client.custom_name || ""); setRenaming(!renaming); }}>
          {client.custom_name ? "Rename" : "Name"}
        </button>
        {client.custom_name && !renaming && (
          <button
            type="button"
            className="quiet-button"
            onClick={async () => {
              try {
                await api.renameNetworkClient(client.key || client.ip, "");
                setRenaming(false);
                onChanged();
              } catch (err) {
                onError((err as Error).message);
              }
            }}
          >
            Reset
          </button>
        )}
      </td>
    </tr>
  );
}
