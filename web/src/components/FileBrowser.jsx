import { useState, useEffect } from 'react';
import { getFiles, deleteFile } from '../services/api';

function formatBytes(bytes) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDate(dateString) {
  return new Date(dateString).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function FileBrowser() {
  const [currentPath, setCurrentPath] = useState('/');
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchFiles(currentPath);
  }, [currentPath]);

  const fetchFiles = async (path) => {
    try {
      setLoading(true);
      setError(null);
      const response = await getFiles(path);
      setFiles(response.data || []);
    } catch (err) {
      setError('Failed to load files');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const navigateTo = (path) => {
    setCurrentPath(path);
  };

  const navigateUp = () => {
    if (currentPath === '/') return;
    const parts = currentPath.split('/').filter(Boolean);
    parts.pop();
    setCurrentPath('/' + parts.join('/'));
  };

  const handleFileClick = (file) => {
    if (file.is_dir) {
      navigateTo(file.virtual_path);
    } else {
      // TODO: Open file preview or download
      console.log('Open file:', file);
    }
  };

  const handleDelete = async (file, e) => {
    e.stopPropagation();
    if (!window.confirm(`Delete ${file.virtual_path}?`)) return;

    try {
      await deleteFile(file.virtual_path);
      setFiles((prev) => prev.filter((f) => f.id !== file.id));
    } catch (err) {
      console.error('Failed to delete file:', err);
    }
  };

  const getFileIcon = (file) => {
    if (file.is_dir) return '📁';

    const ext = file.virtual_path.split('.').pop()?.toLowerCase();
    const icons = {
      mp4: '🎬', mov: '🎬', avi: '🎬', mkv: '🎬',
      mp3: '🎵', wav: '🎵', flac: '🎵',
      jpg: '🖼️', jpeg: '🖼️', png: '🖼️', gif: '🖼️',
      pdf: '📄', doc: '📝', docx: '📝', txt: '📝',
      zip: '📦', rar: '📦', '7z': '📦',
    };
    return icons[ext] || '📄';
  };

  const breadcrumbs = currentPath.split('/').filter(Boolean);

  return (
    <div className="file-browser">
      <div className="breadcrumbs">
        <button onClick={() => navigateTo('/')} className="breadcrumb">
          🏠 Home
        </button>
        {breadcrumbs.map((part, index) => (
          <span key={index}>
            <span className="separator">/</span>
            <button
              onClick={() => navigateTo('/' + breadcrumbs.slice(0, index + 1).join('/'))}
              className="breadcrumb"
            >
              {part}
            </button>
          </span>
        ))}
      </div>

      <div className="file-actions">
        <button onClick={navigateUp} disabled={currentPath === '/'} className="btn">
          ⬆️ Up
        </button>
        <button onClick={() => fetchFiles(currentPath)} className="btn">
          🔄 Refresh
        </button>
      </div>

      {loading ? (
        <div className="loading">Loading files...</div>
      ) : error ? (
        <div className="error">{error}</div>
      ) : files.length === 0 ? (
        <div className="empty-state">
          <p>This folder is empty</p>
        </div>
      ) : (
        <div className="file-list">
          <div className="file-header">
            <span className="col-name">Name</span>
            <span className="col-size">Size</span>
            <span className="col-modified">Modified</span>
            <span className="col-actions">Actions</span>
          </div>
          {files.map((file) => (
            <div
              key={file.id}
              className={`file-row ${file.is_dir ? 'folder' : 'file'}`}
              onClick={() => handleFileClick(file)}
            >
              <span className="col-name">
                <span className="file-icon">{getFileIcon(file)}</span>
                {file.virtual_path.split('/').pop()}
              </span>
              <span className="col-size">
                {file.is_dir ? '—' : formatBytes(file.size_bytes)}
              </span>
              <span className="col-modified">{formatDate(file.updated_at)}</span>
              <span className="col-actions">
                <button
                  className="btn btn-small btn-danger"
                  onClick={(e) => handleDelete(file, e)}
                >
                  🗑️
                </button>
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default FileBrowser;
