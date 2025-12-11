import React from 'react';
import { useAuth } from '../context/AuthContext';

interface GameButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
    children: React.ReactNode;
}

const GameButton: React.FC<GameButtonProps> = ({ children, className, disabled, ...props }) => {
    const { gameLoggedIn } = useAuth();

    // If not logged in, we force disabled state
    const isDisabled = disabled || !gameLoggedIn;

    // If not logged in, show "Start Bot" message
    // We can either replace the content or append/prepend. 
    // Replacing seems cleaner for the specific request "buttons say Start Bot before using this"
    // However, retaining the original intent might be good too. 
    // Let's replace the text if disconnected to be very clear as per user request.
    const buttonContent = !gameLoggedIn ? (
        <span className="flex items-center gap-2">
            <span>Start Bot to {typeof children === 'string' ? 'Enable' : 'Use'}</span>
        </span>
    ) : children;

    return (
        <button
            disabled={isDisabled}
            className={`${className || ''} ${!gameLoggedIn ? 'cursor-not-allowed opacity-50 grayscale' : ''}`.trim()}
            {...props}
        >
            {buttonContent}
        </button>
    );
};

export default GameButton;
