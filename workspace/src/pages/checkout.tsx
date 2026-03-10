import Head from 'next/head';
import { useState } from 'react';
import { Container, Form, Button, Spinner } from '@components';
import { useOrder } from '@hooks';
import { useShipping } from '@hooks';
import { useWindowSize } from '@hooks';

const Checkout = () => {
  const [shippingName, setShippingName] = useState('');
  const [shippingAddress, setShippingAddress] = useState('');
  const [orderTotal, setOrderTotal] = useState(0);

  const { order } = useOrder();
  const { shipping } = useShipping();
  const { windowSize } = useWindowSize();

  const handlePlaceOrder = () => {
    const orderData = {
      ...order,
      shippingName,
      shippingAddress,
    };
    localStorage.setItem('order', JSON.stringify(orderData));
  };

  return (
    <Container>
      <Head>
        <title>Checkout | Example App</title>
      </Head>
      <main className="flex h-screen">
        <aside className="w-64 h-screen p-4 overflow-y-auto">
          <OrderSummary order={order} total={orderTotal} />
        </aside>
        <section className="flex-1 p-4">
          <h1 className="text-3xl font-bold">Shipping Information</h1>
          <Form
            onSubmit={(event) => {
              event.preventDefault();
              handlePlaceOrder();
            }}
          >
            <Form.Group>
              <Form.Label>Full Name:</Form.Label>
              <Form.Control
                type="text"
                value={shippingName}
                onChange={(event) => setShippingName(event.target.value)}
              />
            </Form.Group>
            <Form.Group>
              <Form.Label>Address:</Form.Label>
              <Form.Control
                type="text"
                value={shippingAddress}
                onChange={(event) => setShippingAddress(event.target.value)}
              />
            </Form.Group>
            <Button type="submit" className="w-full">
              {windowSize.width >= 768 ? 'Place Order' : 'Place Order'}{' '}
              {windowSize.width < 768 && (
                <Spinner size="sm" className="ml-2" />
              )}
            </Button>
          </Form>
        </section>
      </main>
    </Container>
  );
};

export default Checkout;