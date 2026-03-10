import { Product } from '../types';

const OrderConfirmation = () => {
  return (
    <div className="max-w-md mx-auto p-4">
      <h1 className="text-3xl font-bold">Order Confirmation</h1>
      <p className="text-lg font-bold">
        Your order has been placed successfully! Thank you for shopping with us.
      </p>
      <ul>
        { /* Add order details here */ }
      </ul>
    </div>
  );
};

export default OrderConfirmation;