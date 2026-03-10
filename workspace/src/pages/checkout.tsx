import { useRouter } from 'next/router';
import { useState } from 'react';
import { Checkout } from '../components/Checkout';

const CheckoutPage = () => {
  const router = useRouter();
  const [orderSummary, setOrderSummary] = useState({});

  const handlePlaceOrder = () => {
    localStorage.setItem('order', JSON.stringify(orderSummary));
    router.push('/order-success');
  };

  return (
    <div className="flex flex-col gap-4">
      <Checkout
        orderSummary={orderSummary}
        setOrderSummary={setOrderSummary}
        handlePlaceOrder={handlePlaceOrder}
      />
      <aside className="w-64 p-4">
        <h2 className="text-lg">Order Summary</h2>
        <ul>
          {Object.keys(orderSummary).map((item) => (
            <li key={item}>{item}: {orderSummary[item]}</li>
          ))}
        </ul>
      </aside>
    </div>
  );
};

export default CheckoutPage;