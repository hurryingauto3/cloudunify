import { useState, useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Link, Navigate } from 'react-router-dom';
import Dashboard from './pages/Dashboard';
import Setup from './pages/Setup';
import Files from './pages/Files';
import Settings from './pages/Settings';
import { getProviders } from './services/api';
import './App.css';

function App() {
  const [isSetupComplete, setIsSetupComplete] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    checkSetupStatus();
  }, []);

  const checkSetupStatus = async () => {
    try {
      const response = await getProviders();
      setIsSetupComplete(response.data && response.data.length > 0);
    } catch {
      // If API fails, assume setup is needed
      setIsSetupComplete(false);
    } finally {
      setLoading(false);
    }
  };

  const handleSetupComplete = () => {
    setIsSetupComplete(true);
  };

  if (loading) {
    return (
      <div className="app-loading">
        <div className="spinner"></div>
        <p>Loading CloudUnify...</p>
      </div>
    );
  }

  if (!isSetupComplete) {
    return <Setup onComplete={handleSetupComplete} />;
  }

  return (
    <Router>
      <div className="app">
        <nav className="sidebar">
          <div className="logo">
            <h1>☁️ CloudUnify</h1>
          </div>
          <ul className="nav-links">
            <li><Link to="/">📊 Dashboard</Link></li>
            <li><Link to="/files">📁 Files</Link></li>
            <li><Link to="/settings">⚙️ Settings</Link></li>
          </ul>
        </nav>
        <main className="main-content">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/files" element={<Files />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;
