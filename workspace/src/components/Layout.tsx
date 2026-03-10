import Head from 'next/head';
import Header from './Header';
import Footer from './Footer';

const Layout = ({ children }: { children: React.ReactNode }) => {
  return (
    <div className="min-h-screen bg-gray-100">
      <Head>
        <title>My App</title>
      </Head>
      <Header />
      <main className="max-w-md mx-auto p-4">{children}</main>
      <Footer />
    </div>
  );
};

export default Layout;