import { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  Grid,
  TextField,
  FormControlLabel,
  Checkbox,
  Button,
  Alert,
  Chip,
  Divider,
  Stack,
} from '@mui/material';
import StorageIcon from '@mui/icons-material/Storage';
import SyncIcon from '@mui/icons-material/Sync';
import TimerIcon from '@mui/icons-material/Timer';
import ReplayIcon from '@mui/icons-material/Replay';
import InfoIcon from '@mui/icons-material/Info';
import { getHealth, getVersion, getConfig, updateConfig } from '../services/api';

function SettingsSection({ icon, title, children }) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <Box sx={{ color: 'primary.main' }}>{icon}</Box>
          <Typography variant="h3">{title}</Typography>
        </Box>
        {children}
      </CardContent>
    </Card>
  );
}

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
      const [healthRes, versionRes] = await Promise.all([getHealth(), getVersion()]);
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
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h1">Settings</Typography>
        <Button variant="contained" onClick={handleSave} disabled={saving || loading}>
          {saving ? 'Saving...' : 'Save Settings'}
        </Button>
      </Box>

      {restartRequired && (
        <Alert severity="warning" sx={{ mb: 3 }}>
          Worker count changed. Restart CloudUnify for changes to take effect.
        </Alert>
      )}

      {/* Storage & Cache Row */}
      <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1.5, textTransform: 'uppercase', letterSpacing: 1 }}>
        Storage
      </Typography>
      <Grid container spacing={2} sx={{ mb: 4 }}>
        <Grid item xs={12} md={6}>
          <SettingsSection icon={<StorageIcon />} title="Mount Point">
            <TextField
              fullWidth
              size="small"
              label="Virtual Drive Location"
              value={settings.mountPath}
              onChange={(e) => handleChange('mountPath', e.target.value)}
              helperText="Where CloudUnify mounts the virtual drive"
            />
          </SettingsSection>
        </Grid>
        <Grid item xs={12} md={6}>
          <SettingsSection icon={<StorageIcon />} title="Cache">
            <FormControlLabel
              control={
                <Checkbox
                  checked={settings.cacheEnabled}
                  onChange={(e) => handleChange('cacheEnabled', e.target.checked)}
                />
              }
              label="Enable local cache"
            />
            <TextField
              fullWidth
              size="small"
              type="number"
              label="Cache Size (GB)"
              value={settings.cacheSizeGB}
              onChange={(e) => handleChange('cacheSizeGB', parseInt(e.target.value))}
              disabled={!settings.cacheEnabled}
              inputProps={{ min: 1, max: 100 }}
              sx={{ mt: 1 }}
            />
          </SettingsSection>
        </Grid>
      </Grid>

      {/* Sync Row */}
      <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1.5, textTransform: 'uppercase', letterSpacing: 1 }}>
        Sync Settings
      </Typography>
      <Grid container spacing={2} sx={{ mb: 4 }}>
        <Grid item xs={12} md={4}>
          <SettingsSection icon={<SyncIcon />} title="Workers">
            <FormControlLabel
              control={
                <Checkbox
                  checked={settings.autoSync}
                  onChange={(e) => handleChange('autoSync', e.target.checked)}
                />
              }
              label="Auto-sync files"
            />
            <Stack direction="row" spacing={2} sx={{ mt: 1 }}>
              <TextField
                fullWidth
                size="small"
                type="number"
                label="Upload"
                value={settings.uploadWorkers}
                onChange={(e) => handleChange('uploadWorkers', parseInt(e.target.value))}
                inputProps={{ min: 1, max: 10 }}
              />
              <TextField
                fullWidth
                size="small"
                type="number"
                label="Download"
                value={settings.downloadWorkers}
                onChange={(e) => handleChange('downloadWorkers', parseInt(e.target.value))}
                inputProps={{ min: 1, max: 10 }}
              />
            </Stack>
            <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
              Restart required after changing worker counts
            </Typography>
          </SettingsSection>
        </Grid>

        <Grid item xs={12} md={4}>
          <SettingsSection icon={<TimerIcon />} title="Timeouts">
            <Stack spacing={2}>
              <TextField
                fullWidth
                size="small"
                type="number"
                label="Download Timeout (sec)"
                value={settings.downloadTimeoutSeconds}
                onChange={(e) =>
                  handleChange('downloadTimeoutSeconds', Math.max(5, Math.min(300, parseInt(e.target.value) || 30)))
                }
                inputProps={{ min: 5, max: 300 }}
                helperText="5-300 seconds"
              />
              <TextField
                fullWidth
                size="small"
                type="number"
                label="Stale Job Timeout (min)"
                value={settings.staleJobTimeoutMinutes}
                onChange={(e) =>
                  handleChange('staleJobTimeoutMinutes', Math.max(5, Math.min(120, parseInt(e.target.value) || 30)))
                }
                inputProps={{ min: 5, max: 120 }}
                helperText="Reset stuck jobs after timeout"
              />
            </Stack>
          </SettingsSection>
        </Grid>

        <Grid item xs={12} md={4}>
          <SettingsSection icon={<ReplayIcon />} title="Retry Policy">
            <Typography variant="caption" color="text.secondary" sx={{ mb: 1.5, display: 'block' }}>
              Max retries per error type (0-10)
            </Typography>
            <Grid container spacing={1}>
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  size="small"
                  type="number"
                  label="Network"
                  value={settings.retryPolicy.networkRetries}
                  onChange={(e) => handleRetryPolicyChange('networkRetries', e.target.value)}
                  inputProps={{ min: 0, max: 10 }}
                />
              </Grid>
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  size="small"
                  type="number"
                  label="Rate Limit"
                  value={settings.retryPolicy.rateLimitRetries}
                  onChange={(e) => handleRetryPolicyChange('rateLimitRetries', e.target.value)}
                  inputProps={{ min: 0, max: 10 }}
                />
              </Grid>
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  size="small"
                  type="number"
                  label="Auth"
                  value={settings.retryPolicy.authRetries}
                  onChange={(e) => handleRetryPolicyChange('authRetries', e.target.value)}
                  inputProps={{ min: 0, max: 10 }}
                />
              </Grid>
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  size="small"
                  type="number"
                  label="Quota"
                  value={settings.retryPolicy.quotaRetries}
                  onChange={(e) => handleRetryPolicyChange('quotaRetries', e.target.value)}
                  inputProps={{ min: 0, max: 10 }}
                />
              </Grid>
            </Grid>
          </SettingsSection>
        </Grid>
      </Grid>

      {/* System Info Row */}
      <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1.5, textTransform: 'uppercase', letterSpacing: 1 }}>
        System
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <SettingsSection icon={<InfoIcon />} title="System Info">
            <Stack spacing={1.5}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">Version</Typography>
                <Typography variant="body2" fontWeight={500}>{version?.version || 'Unknown'}</Typography>
              </Box>
              <Divider />
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">Status</Typography>
                <Chip
                  label={health?.status || 'Unknown'}
                  size="small"
                  color={health?.status === 'healthy' ? 'success' : 'default'}
                />
              </Box>
              <Divider />
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">Mount Status</Typography>
                <Chip
                  label={health?.mount_status || 'Unknown'}
                  size="small"
                  color={health?.mount_status === 'mounted' ? 'success' : 'warning'}
                />
              </Box>
            </Stack>
          </SettingsSection>
        </Grid>
        <Grid item xs={12} md={6}>
          <SettingsSection icon={<TimerIcon />} title="Cleanup">
            <TextField
              fullWidth
              size="small"
              type="number"
              label="Job Retention (hours)"
              value={settings.completedJobRetentionHours}
              onChange={(e) =>
                handleChange('completedJobRetentionHours', Math.max(1, Math.min(168, parseInt(e.target.value) || 24)))
              }
              inputProps={{ min: 1, max: 168 }}
              helperText="How long to keep completed sync jobs in history (1-168 hours)"
            />
          </SettingsSection>
        </Grid>
      </Grid>
    </Box>
  );
}

export default Settings;
