import '../styles/globals.css';
import { AppProps } from 'next/app';
import { validate } from '../api/validate';

function MyApp({ Component, pageProps }: AppProps) {
  return <Component {...pageProps} />;
}

export default validate ? MyApp : (Component: any) => <Component />;