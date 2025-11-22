import React from 'react';
import './Curtain.css'; // We can reuse the curtain styles for the lock page

const LoginPage: React.FC = () => {
  return (
    <div className="curtain-lock">
      <h1>Please login to the game</h1>
      <p>Waiting for successful game client login...</p>
    </div>
  );
};

export default LoginPage;