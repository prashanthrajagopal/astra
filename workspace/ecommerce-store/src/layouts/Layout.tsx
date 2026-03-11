import Header from '../components/Header';
import Footer from '../components/Footer';

interface Props {
  children: React.ReactNode;
}

const Layout: React.FC<Props> = ({ children }) => {
  return (
    <div className="h-screen overflow-y-scroll">
      <Header itemCount={1} /> /* pass itemCount prop here */
      <main>{children}</main>
      <Footer />
    </div>
  );
};

export default Layout;