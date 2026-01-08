import SetupWizard from '../components/SetupWizard';

function Setup({ onComplete }) {
  return (
    <div className="setup-page">
      <SetupWizard onComplete={onComplete} />
    </div>
  );
}

export default Setup;
