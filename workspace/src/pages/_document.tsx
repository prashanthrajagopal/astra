import { Html, Head, Main, NextScript } from 'next';
import { CartProvider } from '../context/CartContext';

function MyApp({ children }) {
  return (
    <CartProvider>
      <Html>
        <Head>
          <title>My E-commerce App</title>
        </Head>
        <body>
          <Main>{children}</Main>
          <NextScript />
        </body>
      </Html>
    </CartProvider>
  );
}

export default MyApp;