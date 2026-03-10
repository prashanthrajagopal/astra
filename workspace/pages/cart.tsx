import { Layout } from '../components/Layout';
import { CartList } from '../components/CartList';
import { CartSummary } from '../components/CartSummary';

const CartPage = () => {
  return (
    <Layout>
      <CartList />
      <CartSummary />
    </Layout>
  );
};

export default CartPage;