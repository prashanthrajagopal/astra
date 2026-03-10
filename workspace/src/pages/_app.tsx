import type { NextPage } from 'next';
import type { AppProps } from 'next/app';
import { App } from 'next/app';
import { ValidationProvider } from '../components/validation/ValidationProvider';
import { ValidationEditor } from '../components/validation/ValidationEditor';

const MyApp = ({ Component, pageProps }: AppProps) => {
  return (
    <App>
      <ValidationProvider>
        <Component {...pageProps} />
      </ValidationProvider>
    </App>
  );
};

export default MyApp;