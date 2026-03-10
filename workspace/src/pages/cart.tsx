import Head from 'next/head';
import Cart from '../components/Cart';

const CartPage = () => {
  return (
    <div>
      <Head>
        <title>Shopping Cart</title>
      </Head>
      <Cart />
    </div>
  );
};

export default CartPage;