import { ReactNode } from 'react';
import '../global.scss';

export default function OAuthLayout({ children }: { children: ReactNode }) {
  return (
    <html>
      <body style={{ background: '#0e0e0e', color: '#fff', fontFamily: 'system-ui' }}>
        {children}
      </body>
    </html>
  );
}
