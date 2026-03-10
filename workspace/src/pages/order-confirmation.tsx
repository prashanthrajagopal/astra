import Head from 'next/head';
import { useState, useEffect } from 'react';
import { useOrder } from '../hooks/useOrder';
import { useOrderItems } from '../hooks/useOrderItems';
import { useOrderTotal } from '../hooks/useOrderTotal';
import { useOrderStatus } from '../hooks/useOrderStatus';
import OrderSummary from '../components/OrderSummary';
import OrderDetails from '../components/OrderDetails';
import OrderStatus from '../components/OrderStatus';
import { Container, Heading, Text } from '@components';

const OrderConfirmation = () => {
  const { order } = useOrder();
  const { orderItems } = useOrderItems();
  const { orderTotal } = useOrderTotal();
  const { orderStatus } = useOrderStatus();

  return (
    <Container className="pt-20">
      <Head>
        <title>Order Confirmation - {order?.name}</title>
      </Head>
      <Heading as="h1" size="lg">
        Order Confirmation
      </Heading>
      <OrderSummary
        orderItems={orderItems}
        orderTotal={orderTotal}
      />
      <OrderDetails
        order={order}
        orderItems={orderItems}
      />
      <OrderStatus status={orderStatus} />
    </Container>
  );
};

export default OrderConfirmation;