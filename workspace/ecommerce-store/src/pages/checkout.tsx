import Head from 'next/head';
import { useState } from 'react';
import ShippingInformationForm from '../components/ShippingInformationForm';
import OrderSummary from '../components/OrderSummary';

function Checkout() {
  const [shippingInformation, setShippingInformation] = useState({
    name: '',
    email: '',
    address: '',
    city: '',
    zip: '',
  });

  const [orderSummary, setOrderSummary] = useState({
    subtotal: 0,
    tax: 0,
    total: 0,
  });

  const handleFormSubmit = () => {
    localStorage.setItem('order', JSON.stringify(orderSummary));
  };

  return (
    <div className="flex flex-col h-screen">
      <Head>
        <title>Checkout</title>
      </Head>
      <main className="flex-grow">
        <ShippingInformationForm
          shippingInformation={shippingInformation}
          setShippingInformation={setShippingInformation}
        />
        <OrderSummary orderSummary={orderSummary} />
        <button
          className="bg-orange-500 hover:bg-orange-700 text-white font-bold py-2 px-4 rounded"
          onClick={handleFormSubmit}
        >
          Place Order
        </button>
      </main>
    </div>
  );
}

export default Checkout;