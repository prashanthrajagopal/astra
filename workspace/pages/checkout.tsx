import { Layout } from '../components/Layout';
import { CheckoutForm } from '../components/CheckoutForm';
import { OrderSummary } from '../components/OrderSummary';

const CheckoutPage = () => {
  return (
    <Layout>
      <CheckoutForm />
      <OrderSummary />
    </Layout>
  );
};

export default CheckoutPage;