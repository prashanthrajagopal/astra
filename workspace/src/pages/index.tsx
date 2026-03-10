import Head from 'next/head';
import { useRouter } from 'next/router';
import Home from '../components/Home';

const IndexPage = () => {
  const router = useRouter();

  return (
    <div>
      <Head>
        <title>Home Page</title>
      </Head>
      <Home />
    </div>
  );
};

export default IndexPage;