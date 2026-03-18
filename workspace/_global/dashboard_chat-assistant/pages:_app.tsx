import { AnalyticsProvider } from '../components/AnalyticsProvider';
import '../styles/globals.css';

function MyApp({ Component, pageProps }) {
  return (
    <AnalyticsProvider>
      <Component {...pageProps} />
    </AnalyticsProvider>
  );
}

export default MyApp;