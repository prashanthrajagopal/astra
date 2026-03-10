import { useState, useEffect } from 'react';
import { useRouter } from 'next/router';
import { useCart } from '../hooks/useCart';
import { CartItem } from '../types';

const Cart = () => {
  const { cartItems, updateCart } = useCart();
  const router = useRouter();
  const [subtotal, setSubtotal] = useState(0);
  const [total, setTotal] = useState(0);

  useEffect(() => {
    setSubtotal(cartItems.reduce((acc, item) => acc + item.price * item.quantity, 0));
    setTotal(subtotal + 10); // assume $10 shipping cost
  }, [cartItems]);

  const handleQuantityChange = (id: string, quantity: number) => {
    updateCart({ ...cartItems.find((item) => item.id === id), quantity });
  };

  const handleRemove = (id: string) => {
    updateCart(cartItems.filter((item) => item.id !== id));
  };

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-3xl">Shopping Cart</h2>
      <ul className="flex flex-col gap-4">
        {cartItems.map((item) => (
          <li key={item.id} className="flex justify-between">
            <span>{item.name}</span>
            <span className="font-bold">{item.price} x {item.quantity} = {item.price * item.quantity}</span>
            <div className="flex gap-2">
              <button onClick={() => handleQuantityChange(item.id, item.quantity - 1)}>-</button>
              <span>{item.quantity}</span>
              <button onClick={() => handleQuantityChange(item.id, item.quantity + 1)}>+</button>
              <button onClick={() => handleRemove(item.id)}>Remove</button>
            </div>
          </li>
        ))}
      </ul>
      <div className="flex justify-between">
        <span>Subtotal: ${subtotal}</span>
        <span>Total: ${total}</span>
        <button onClick={() => router.push('/checkout')}>Proceed to Checkout</button>
      </div>
    </div>
  );
};

export default Cart;