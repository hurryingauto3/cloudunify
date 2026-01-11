import axios from 'axios';

// Use relative URL so Vite proxy works in dev, and absolute in production
const API_BASE_URL = '/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Provider endpoints
export const getProviders = () => api.get('/providers');
export const addProvider = (type, name = '') => api.post('/providers', { type, name: name || type });
export const deleteProvider = (id) => api.delete(`/providers/${id}`);
export const getProviderQuota = (id) => api.get(`/providers/${id}/quota`);
export const refreshProviderToken = (id) => api.post(`/providers/${id}/refresh`);
export const verifyICloud = (id, customPath = '') => api.post(`/providers/${id}/verify-icloud`, { custom_path: customPath });

// OAuth endpoints
export const getOAuthStatus = () => api.get('/auth/status');
export const getAuthURL = (providerType, providerId = null) => {
  const params = providerId ? `?provider_id=${providerId}` : '';
  return api.get(`/auth/${providerType}/url${params}`);
};

// Storage endpoints
export const getStorageStats = () => api.get('/storage');
export const getStorageUsage = () => api.get('/storage/usage');

// File endpoints
export const getFiles = (path = '/') => api.get('/files', { params: { path } });
export const getFileMetadata = (path) => api.get(`/files/${encodeURIComponent(path)}`);
export const deleteFile = (path) => api.delete(`/files/${encodeURIComponent(path)}`);
export const searchFiles = (query, limit = 50) => api.post('/files/search', { query, limit });
export const pinFile = (id) => api.post(`/files/${id}/pin`);
export const unpinFile = (id) => api.post(`/files/${id}/unpin`);
export const dehydrateFile = (id) => api.post(`/files/${id}/dehydrate`);

// Sync endpoints
export const getSyncQueue = () => api.get('/sync/queue');
export const getSyncStatus = () => api.get('/sync/status');
export const pauseSync = () => api.post('/sync/pause');
export const resumeSync = () => api.post('/sync/resume');
export const cancelSyncItem = (id) => api.delete(`/sync/queue/${id}`);

// Config endpoints
export const getConfig = () => api.get('/config');
export const updateConfig = (config) => api.put('/config', config);

// System endpoints
export const getHealth = () => api.get('/health');
export const getVersion = () => api.get('/version');
export const shutdown = () => api.post('/shutdown');

export default api;
