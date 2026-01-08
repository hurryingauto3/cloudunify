import { useState } from 'react';
import { addProvider } from '../services/api';

const providerOptions = [
  {
    type: 'google_drive',
    name: 'Google Drive',
    icon: '📁',
    description: 'Connect your Google Drive account',
    quota: '2 TB',
  },
  {
    type: 'onedrive',
    name: 'OneDrive',
    icon: '☁️',
    description: 'Connect your Microsoft OneDrive account',
    quota: '1 TB',
  },
  {
    type: 'icloud',
    name: 'iCloud',
    icon: '🍎',
    description: 'Connect your Apple iCloud account',
    quota: '2 TB',
  },
];

function SetupWizard({ onComplete }) {
  const [step, setStep] = useState(1);
  const [selectedProviders, setSelectedProviders] = useState([]);
  const [connecting, setConnecting] = useState(false);
  const [connectedProviders, setConnectedProviders] = useState([]);
  const [error, setError] = useState(null);

  const toggleProvider = (type) => {
    setSelectedProviders((prev) =>
      prev.includes(type)
        ? prev.filter((p) => p !== type)
        : [...prev, type]
    );
  };

  const connectProvider = async (type) => {
    setConnecting(true);
    setError(null);

    try {
      const response = await addProvider(type);
      // In a real app, this would open an OAuth popup
      // For now, we'll just mark it as connected
      if (response.data.auth_url) {
        window.open(response.data.auth_url, '_blank', 'width=600,height=600');
      }
      setConnectedProviders((prev) => [...prev, type]);
    } catch (err) {
      setError(`Failed to connect ${type}: ${err.message}`);
    } finally {
      setConnecting(false);
    }
  };

  const handleNext = () => {
    if (step === 1 && selectedProviders.length > 0) {
      setStep(2);
    } else if (step === 2) {
      setStep(3);
    }
  };

  const handleComplete = () => {
    onComplete();
  };

  return (
    <div className="setup-wizard">
      <div className="wizard-header">
        <h1>Welcome to CloudUnify</h1>
        <p>Unify your cloud storage into one virtual drive</p>
      </div>

      <div className="wizard-progress">
        <div className={`step ${step >= 1 ? 'active' : ''}`}>1. Select Providers</div>
        <div className={`step ${step >= 2 ? 'active' : ''}`}>2. Connect</div>
        <div className={`step ${step >= 3 ? 'active' : ''}`}>3. Ready</div>
      </div>

      {step === 1 && (
        <div className="wizard-content">
          <h2>Select your cloud storage providers</h2>
          <p>Choose which services you want to unify</p>

          <div className="provider-options">
            {providerOptions.map((provider) => (
              <div
                key={provider.type}
                className={`provider-option ${selectedProviders.includes(provider.type) ? 'selected' : ''
                  }`}
                onClick={() => toggleProvider(provider.type)}
              >
                <span className="provider-icon">{provider.icon}</span>
                <div className="provider-details">
                  <h3>{provider.name}</h3>
                  <p>{provider.description}</p>
                  <span className="quota-badge">{provider.quota}</span>
                </div>
                <div className="checkbox">
                  {selectedProviders.includes(provider.type) ? '✓' : ''}
                </div>
              </div>
            ))}
          </div>

          <div className="wizard-actions">
            <button
              className="btn btn-primary"
              onClick={handleNext}
              disabled={selectedProviders.length === 0}
            >
              Continue
            </button>
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="wizard-content">
          <h2>Connect your accounts</h2>
          <p>Sign in to each service to grant CloudUnify access</p>

          {error && <div className="error">{error}</div>}

          <div className="connect-list">
            {selectedProviders.map((type) => {
              const provider = providerOptions.find((p) => p.type === type);
              const isConnected = connectedProviders.includes(type);

              return (
                <div key={type} className="connect-item">
                  <span className="provider-icon">{provider.icon}</span>
                  <span className="provider-name">{provider.name}</span>
                  {isConnected ? (
                    <span className="status connected">✓ Connected</span>
                  ) : (
                    <button
                      className="btn btn-secondary"
                      onClick={() => connectProvider(type)}
                      disabled={connecting}
                    >
                      {connecting ? 'Connecting...' : 'Connect'}
                    </button>
                  )}
                </div>
              );
            })}
          </div>

          <div className="wizard-actions">
            <button className="btn" onClick={() => setStep(1)}>
              Back
            </button>
            <button
              className="btn btn-primary"
              onClick={handleNext}
              disabled={connectedProviders.length === 0}
            >
              Continue
            </button>
          </div>
        </div>
      )}

      {step === 3 && (
        <div className="wizard-content">
          <div className="success-icon">🎉</div>
          <h2>You're all set!</h2>
          <p>
            CloudUnify is ready to use. Your unified storage is mounted at{' '}
            <code>~/CloudUnify</code>
          </p>

          <div className="summary">
            <h3>Connected Providers</h3>
            <ul>
              {connectedProviders.map((type) => {
                const provider = providerOptions.find((p) => p.type === type);
                return (
                  <li key={type}>
                    {provider.icon} {provider.name}
                  </li>
                );
              })}
            </ul>
          </div>

          <div className="wizard-actions">
            <button className="btn btn-primary" onClick={handleComplete}>
              Go to Dashboard
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export default SetupWizard;
