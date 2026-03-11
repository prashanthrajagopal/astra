import Head from 'next/head';
import { useState } from 'react';
import clsx from 'clsx';
import styles from './order-confirmation.module.css';

const OrderConfirmation = () => {
  const [order, setOrder] = useState({
    id: '1234567890',
    date: '2023-03-17T14:30:00.000Z',
    total: 59.99,
    items: [
      { id: 'prod1', name: 'Product 1', quantity: 2 },
      { id: 'prod2', name: 'Product 2', quantity: 1 },
    ],
  });

  return (
    <div className={clsx('container', styles.orderConfirmation)}>
      <Head>
        <title>Order Confirmation</title>
      </Head>
      <h1 className={styles.header}>Order Confirmation</h1>
      <ul className={styles.orderSummary}>
        <li>
          <span className={styles.summaryItem}>Order ID:</span>
          {order.id}
        </li>
        <li>
          <span className={styles.summaryItem}>Order Date:</span>
          {order.date}
        </li>
        <li>
          <span className={styles.summaryItem}>Total:</span>
          ${order.total.toFixed(2)}
        </li>
      </ul>
      <ul className={styles.orderItems}>
        {order.items.map((item) => (
          <li key={item.id} className={styles.orderItem}>
            <span className={styles.itemName}>{item.name}</span>
            <span className={styles.itemQuantity}>
              x{item.quantity}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
};

export default OrderConfirmation;