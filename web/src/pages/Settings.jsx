import { useState, useEffect, useCallback } from 'react';
import { getHealth, getVersion } from '../services/api';

function Settings() {
  const [health, setHealth] = useState(null);
  const [version, setVersion] = useState(null);
  const [settings, setSettings] = useState({
    mountPath: '~/CloudUnify',
    cacheEnabled: true,
    cacheSizeGB: 10,
    uploadWorkers: 3,
    downloadWorkers: 5,
    autoSync: true,
  });

  const fetchSystemInfo = useCallback(async () => {
    try {
      const [healthRes, versionRes] = await Promise.all([
        getHealth(),
        getVersion(),
      ]);
      setHealth(healthRes.data);
      setVersion(versionRes.data);
    } catch (err) {
      console.error('Failed to fetch system info:', err);
    }
  }, []);

  useEffect(() => {
    fetchSystemInfo();
  }, [fetchSystemInfo]);

  const handleChange = (key, value) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
  };

  const handleSave = () => {
    // TODO: Save settings via API
    console.log('Saving settings:', settings);
  };

  return (
    <div className="settings-page">
      <h1>Settings</h1>

      <section className="settings-section">
        <h2>Mount Point</h2>
        <div className="setting-item">
          <label>Virtual Drive Location</label>
          <input
            type="text"
            value={settings.mountPath}
            onChange={(e) => handleChange('mountPath', e.target.value)}
          />
          <p className="hint">The location where CloudUnify will mount the virtual drive</p>
        </div>
      </section>

      <section className="settings-section">
        <h2>Cache Settings</h2>
        <div className="setting-item">
          <label>
            <input
              type="checkbox"
              checked={settings.cacheEnabled}
              onChange={(e) => handleChange('cacheEnabled', e.target.checked)}
            />
            Enable local cache
          </label>
          <p className="hint">Cache frequently accessed files for faster access</p>
        </div>
        <div className="setting-item">
          <label>Cache Size (GB)</label>
          <input
            type="number"
            min="1"
            max="100"
            value={settings.cacheSizeGB}
            onChange={(e) => handleChange('cacheSizeGB', parseInt(e.target.value))}
            disabled={!settings.cacheEnabled}
          />
        </div>
      </section>

      <section className="settings-section">
        <h2>Sync Settings</h2>
        <div className="setting-item">
          <label>
            <input
              type="checkbox"
              checked={settings.autoSync}
              onChange={(e) => handleChange('autoSync', e.target.checked)}
            />
            Auto-sync files
          </label>
          <p className="hint">Automatically sync changes in the background</p>
        </div>
        <div className="setting-item">
          <label>Upload Workers</label>
          <input
            type="number"
            min="1"
            max="10"
            value={settings.uploadWorkers}
            onChange={(e) => handleChange('uploadWorkers', parseInt(e.target.value))}
          />
        </div>
        <div className="setting-item">
          <label>Download Workers</label>
          <input
            type="number"
            min="1"
            max="10"
            value={settings.downloadWorkers}
            onChange={(e) => handleChange('downloadWorkers', parseInt(e.target.value))}
          />
        </div>
      </section>

      <section className="settings-section">
        <h2>System Information</h2>
        <div className="system-info">
          <p><strong>Version:</strong> {version?.version || 'Unknown'}</p>
          <p><strong>Status:</strong> {health?.status || 'Unknown'}</p>
          <p><strong>Mount Status:</strong> {health?.mount_status || 'Unknown'}</p>
        </div>
      </section>

      <div className="settings-actions">
        <button className="btn btn-primary" onClick={handleSave}>
          Save Settings
        </button>
      </div>
    </div>
  );
}

export default Settings;
