import Head from 'next/head';
import { useRouter } from 'next/router';
import { useState, useEffect } from 'react';
import { Order } from '../models/Order';
import styles from './order-confirmation.module.css';

interface OrderConfirmationProps {
  order: Order;
}

const OrderConfirmation = ({ order }: OrderConfirmationProps) => {
  const router = useRouter();
  const [orderDetails, setOrderDetails] = useState(order);

  useEffect(() => {
    setOrderDetails(order);
  }, [order]);

  return (
    <div className={styles.container}>
      <Head>
        <title>Order Confirmation</title>
      </Head>
      <h1 className={styles.title}>Order Confirmation</h1>
      <ul className={styles.orderDetails}>
        <li>
          <span className={styles.label}>Order Number:</span>
          <span className={styles.value}>{orderDetails.orderNumber}</span>
        </li>
        <li>
          <span className={styles.label}>Total:</span>
          <span className={styles.value}>{orderDetails.total}</span>
        </li>
        <li>
          <span className={styles.label}>Items:</span>
          <ul>
            {orderDetails.items.map((item) => (
              <li key={item.id}>
                <span className={styles.itemName}>{item.name}</span>
                <span className={styles.itemQuantity}>{item.quantity}</span>
              </li>
            ))}
          </ul>
        </li>
      </ul>
      <button
        className={styles.backToShop}
        onClick={() => router.push('/')}
      >
        Back to Shop
      </button>
    </div>
  );
};

export default OrderConfirmation;