'use client';

import { useEffect, useState } from 'react';
import { useSearchParams, usePathname } from 'next/navigation';

export default function OAuthCallbackPage() {
  const searchParams = useSearchParams();
  const pathname = usePathname();
  const [status, setStatus] = useState('Connecting...');

  useEffect(() => {
    const provider = pathname.split('/').pop() || '';
    const code = searchParams.get('code') || '';
    const state = searchParams.get('state') || '';

    if (!code || !state) {
      setStatus('Missing code or state');
      return;
    }

    // Call social-connect API directly (public endpoint, no auth needed)
    fetch(`/s/api/integrations/social-connect/${provider}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code, state }),
    })
      .then(res => res.json())
      .then(data => {
        if (data.msg) {
          setStatus(`Error: ${data.msg}`);
          return;
        }
        setStatus('Connected! Closing...');
        // Reload parent iframe and close popup
        try { window.opener?.location?.reload(); } catch {}
        setTimeout(() => window.close(), 1000);
      })
      .catch(err => setStatus(`Error: ${err.message}`));
  }, [searchParams, pathname]);

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}>
      <p>{status}</p>
    </div>
  );
}
