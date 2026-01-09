import { useState, useEffect, useCallback } from 'react';
import { getHealth, getVersion, getConfig, updateConfig } from '../services/api';

function Settings() {
  const [health, setHealth] = useState(null);
  const [version, setVersion] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [restartRequired, setRestartRequired] = useState(false);
  const [settings, setSettings] = useState({
    mountPath: '~/CloudUnify',
    cacheEnabled: true,
    cacheSizeGB: 10,
    uploadWorkers: 3,
    downloadWorkers: 5,
    autoSync: true,
    // Advanced sync settings
    downloadTimeoutSeconds: 30,
    completedJobRetentionHours: 24,
    staleJobTimeoutMinutes: 30,
    retryPolicy: {
      networkRetries: 5,
      authRetries: 1,
      quotaRetries: 0,
      rateLimitRetries: 5,
    },
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

  const fetchConfig = useCallback(async () => {
    try {
      const res = await getConfig();
      const cfg = res.data;
      setSettings({
        mountPath: cfg.mount_path || '~/CloudUnify',
        cacheEnabled: cfg.cache?.enabled ?? true,
        cacheSizeGB: cfg.cache?.max_size_gb || 10,
        uploadWorkers: cfg.sync?.upload_workers || 3,
        downloadWorkers: cfg.sync?.download_workers || 5,
        autoSync: cfg.sync?.auto_sync ?? true,
        downloadTimeoutSeconds: cfg.sync?.download_timeout_seconds || 30,
        completedJobRetentionHours: cfg.sync?.completed_job_retention_hours || 24,
        staleJobTimeoutMinutes: cfg.sync?.stale_job_timeout_minutes || 30,
        retryPolicy: {
          networkRetries: cfg.sync?.retry_policy?.network_retries ?? 5,
          authRetries: cfg.sync?.retry_policy?.auth_retries ?? 1,
          quotaRetries: cfg.sync?.retry_policy?.quota_retries ?? 0,
          rateLimitRetries: cfg.sync?.retry_policy?.rate_limit_retries ?? 5,
        },
      });
    } catch (err) {
      console.error('Failed to fetch config:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSystemInfo();
    fetchConfig();
  }, [fetchSystemInfo, fetchConfig]);

  const handleChange = (key, value) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
    setRestartRequired(false);
  };

  const handleRetryPolicyChange = (key, value) => {
    // Clamp value to 0-10
    const clampedValue = Math.max(0, Math.min(10, parseInt(value) || 0));
    setSettings((prev) => ({
      ...prev,
      retryPolicy: { ...prev.retryPolicy, [key]: clampedValue },
    }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const res = await updateConfig({
        sync: {
          upload_workers: settings.uploadWorkers,
          download_workers: settings.downloadWorkers,
          auto_sync: settings.autoSync,
          download_timeout_seconds: settings.downloadTimeoutSeconds,
          completed_job_retention_hours: settings.completedJobRetentionHours,
          stale_job_timeout_minutes: settings.staleJobTimeoutMinutes,
          retry_policy: {
            network_retries: settings.retryPolicy.networkRetries,
            auth_retries: settings.retryPolicy.authRetries,
            quota_retries: settings.retryPolicy.quotaRetries,
            rate_limit_retries: settings.retryPolicy.rateLimitRetries,
          },
        },
      });
      if (res.data?.restart_required) {
        setRestartRequired(true);
      }
    } catch (err) {
      console.error('Failed to save settings:', err);
    } finally {
      setSaving(false);
    }
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
          <p className="hint">Requires restart to take effect</p>
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
          <p className="hint">Requires restart to take effect</p>
        </div>
      </section>

      <section className="settings-section">
        <h2>Advanced Sync Settings</h2>
        <div className="setting-item">
          <label>Download Timeout (seconds)</label>
          <input
            type="number"
            min="5"
            max="300"
            value={settings.downloadTimeoutSeconds}
            onChange={(e) => handleChange('downloadTimeoutSeconds', Math.max(5, Math.min(300, parseInt(e.target.value) || 30)))}
          />
          <p className="hint">Maximum time to wait for file downloads (5-300 seconds)</p>
        </div>
        <div className="setting-item">
          <label>Job Retention (hours)</label>
          <input
            type="number"
            min="1"
            max="168"
            value={settings.completedJobRetentionHours}
            onChange={(e) => handleChange('completedJobRetentionHours', Math.max(1, Math.min(168, parseInt(e.target.value) || 24)))}
          />
          <p className="hint">How long to keep completed jobs in the queue (1-168 hours)</p>
        </div>
        <div className="setting-item">
          <label>Stale Job Timeout (minutes)</label>
          <input
            type="number"
            min="5"
            max="120"
            value={settings.staleJobTimeoutMinutes}
            onChange={(e) => handleChange('staleJobTimeoutMinutes', Math.max(5, Math.min(120, parseInt(e.target.value) || 30)))}
          />
          <p className="hint">Jobs stuck processing longer than this are reset (5-120 minutes)</p>
        </div>
      </section>

      <section className="settings-section">
        <h2>Retry Policy</h2>
        <p className="section-hint">Configure how many times to retry failed operations (0-10)</p>
        <div className="setting-item">
          <label>Network Errors</label>
          <input
            type="number"
            min="0"
            max="10"
            value={settings.retryPolicy.networkRetries}
            onChange={(e) => handleRetryPolicyChange('networkRetries', e.target.value)}
          />
          <p className="hint">Connection timeouts, DNS failures, etc.</p>
        </div>
        <div className="setting-item">
          <label>Rate Limit Errors</label>
          <input
            type="number"
            min="0"
            max="10"
            value={settings.retryPolicy.rateLimitRetries}
            onChange={(e) => handleRetryPolicyChange('rateLimitRetries', e.target.value)}
          />
          <p className="hint">API throttling from cloud providers</p>
        </div>
        <div className="setting-item">
          <label>Auth Errors</label>
          <input
            type="number"
            min="0"
            max="10"
            value={settings.retryPolicy.authRetries}
            onChange={(e) => handleRetryPolicyChange('authRetries', e.target.value)}
          />
          <p className="hint">Token expiry, authorization failures</p>
        </div>
        <div className="setting-item">
          <label>Quota Errors</label>
          <input
            type="number"
            min="0"
            max="10"
            value={settings.retryPolicy.quotaRetries}
            onChange={(e) => handleRetryPolicyChange('quotaRetries', e.target.value)}
          />
          <p className="hint">Storage quota exceeded (usually not retriable)</p>
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

      {restartRequired && (
        <div className="alert alert-warning">
          ⚠️ Worker count changed. Restart CloudUnify for changes to take effect.
        </div>
      )}

      <div className="settings-actions">
        <button className="btn btn-primary" onClick={handleSave} disabled={saving || loading}>
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  );
}

export default Settings;
