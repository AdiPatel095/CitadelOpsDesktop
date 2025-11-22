import { AuthProvider, useAuth } from './AuthContext';
import LoginPage from './LoginPage';
import Dashboard from './Dashboard';
import './App.css';
import './Curtain.css';

const AppContent: React.FC = () => {
  const { isLocked } = useAuth();

  // Conditionally render the LoginPage or the main Dashboard
  return isLocked ? <LoginPage /> : <Dashboard />;
};

function App() {
  return (
    <AuthProvider>
      <AppContent />
    </AuthProvider>
  );
}

export default App;
