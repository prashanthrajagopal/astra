import { useState, useEffect } from 'react';
import { useRouter } from 'next/router';
import { useCart } from '../hooks/useCart';
import { formatCurrency } from '../utils/formatters';
import { CartItem } from '../components/CartItem';

const Cart = () => {
  const { cartItems, updateCart, removeItem } = useCart();
  const router = useRouter();
  const [subtotal, setSubtotal] = useState(0);
  const [tax, setTax] = useState(0);
  const [total, setTotal] = useState(0);

  useEffect(() => {
    setSubtotal(cartItems.reduce((acc, item) => acc + item.price * item.quantity, 0));
    setTax(subtotal * 0.08);
    setTotal(subtotal + tax);
  }, [cartItems]);

  return (
    <div className="bg-white py-4">
      <h2 className="text-3xl font-bold">Your Cart</h2>
      <ul>
        {cartItems.map((item) => (
          <CartItem
            key={item.id}
            item={item}
            quantity={item.quantity}
            onQuantityChange={(newQuantity) => updateCart(item.id, newQuantity)}
            onDelete={() => removeItem(item.id)}
          />
        ))}
      </ul>
      <div className="bg-gray-100 py-4">
        <p className="text-lg font-bold">Subtotal: {formatCurrency(subtotal)}</p>
        <p className="text-lg font-bold">Tax: {formatCurrency(tax)}</p>
        <p className="text-lg font-bold">Total: {formatCurrency(total)}</p>
      </div>
    </div>
  );
};

export default Cart;