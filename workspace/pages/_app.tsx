import { AppProps } from 'next/app';
import { validatePhase1 } from '../utils/validatePhase1';

function MyApp({ Component, pageProps }: AppProps) {
  const validationStatus = validatePhase1();

  return (
    <Component {...pageProps} validationStatus={validationStatus} />
  );
}

export default MyApp;