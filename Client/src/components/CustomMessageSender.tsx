import React, { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { Card, CardHeader, CardTitle, CardContent, Input, Button } from './ui';
import { Icons } from './Icons';

const CustomMessageSender: React.FC = () => {
  const [messageCode, setMessageCode] = useState('');
  const { sendMessage } = useAuth();

  const handleSend = () => {
    if (!messageCode.trim()) return;

    sendMessage('sendCustomMessage', {
      messageCode: messageCode.trim()
    });

    setMessageCode('');
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      handleSend();
    }
  };

  return (
    <Card className="liquid-prominent-header-card w-full mb-6">
      <CardHeader className="liquid-card-header-prominent">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center">
            <svg className="w-4 h-4 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
            </svg>
          </div>
          <CardTitle className="text-lg">Send Custom Message</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content p-6">
        <div className="flex gap-3">
          <div className="flex-1">
            <Input
              type="text"
              value={messageCode}
              onChange={(e) => setMessageCode(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="e.g. lli, ggm"
              maxLength={10}
              className="font-mono text-base py-2.5"
            />
          </div>
          <Button
            variant="primary"
            onClick={handleSend}
            disabled={!messageCode.trim()}
            className="px-8 bg-indigo-600 hover:bg-indigo-700 text-white shadow-lg shadow-indigo-600/20 hover:shadow-indigo-600/40 border-indigo-600"
            leftIcon={<Icons.ArrowRight className="w-4 h-4" />}
          >
            Send
          </Button>
        </div>
        <p className="text-xs text-text-muted mt-3 font-medium flex items-center gap-1.5">
          <Icons.Info className="w-4 h-4" />
          Sends a raw formatted message (%%xt%%EmpireEx_XXX%%[code]%%1%%{`{ }`}%%) directly to the game server.
        </p>
      </CardContent>
    </Card>
  );
};

export default CustomMessageSender;
