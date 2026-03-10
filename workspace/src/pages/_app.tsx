import { AppProps } from 'next/app';
import { SessionProvider } from 'next-auth/react';
import { theme } from '../styles/theme';

function MyApp({ Component, pageProps }: AppProps) {
  return (
    <SessionProvider>
      <div className={theme.bgDefault}>
        <Component {...pageProps} />
      </div>
    </SessionProvider>
  );
}

export default MyApp;