import axios from 'axios';

const API_BASE_URL = 'http://localhost:8080/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Provider endpoints
export const getProviders = () => api.get('/providers');
export const addProvider = (type) => api.post('/providers', { type });
export const deleteProvider = (id) => api.delete(`/providers/${id}`);
export const getProviderQuota = (id) => api.get(`/providers/${id}/quota`);
export const refreshProviderToken = (id) => api.post(`/providers/${id}/refresh`);

// Storage endpoints
export const getStorageStats = () => api.get('/storage');
export const getStorageUsage = () => api.get('/storage/usage');

// File endpoints
export const getFiles = (path = '/') => api.get('/files', { params: { path } });
export const getFileMetadata = (path) => api.get(`/files/${encodeURIComponent(path)}`);
export const deleteFile = (path) => api.delete(`/files/${encodeURIComponent(path)}`);
export const searchFiles = (query) => api.post('/files/search', { query });

// Sync endpoints
export const getSyncQueue = () => api.get('/sync/queue');
export const getSyncStatus = () => api.get('/sync/status');
export const pauseSync = () => api.post('/sync/pause');
export const resumeSync = () => api.post('/sync/resume');
export const cancelSyncItem = (id) => api.delete(`/sync/queue/${id}`);

// System endpoints
export const getHealth = () => api.get('/health');
export const getVersion = () => api.get('/version');
export const shutdown = () => api.post('/shutdown');

export default api;
