import { useState } from 'react';
import { CartProvider } from './cart/CartContext';
import Header from './Header';
import Footer from './Footer';

const Layout = () => {
  const [searchTerm, setSearchTerm] = useState('');

  return (
    <CartProvider>
      <Header searchTerm={searchTerm} setSearchTerm={setSearchTerm} />
      <main>
        {/* page content */}
      </main>
      <Footer />
    </CartProvider>
  );
};

export default Layout;