import { useState } from 'react';

interface OrderSummary {
  subtotal: number;
  tax: number;
  total: number;
}

const OrderSummary = ({ orderSummary }: { orderSummary: OrderSummary }) => {
  return (
    <aside className="flex flex-col gap-4 bg-gray-200 p-4">
      <h2 className="text-lg font-bold">Order Summary</h2>
      <ul className="list-disc">
        <li>Subtotal: ${orderSummary.subtotal}</li>
        <li>Tax: ${orderSummary.tax}</li>
        <li>Total: ${orderSummary.total}</li>
      </ul>
    </aside>
  );
};

export default OrderSummary;