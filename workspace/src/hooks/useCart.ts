import { useContext } from 'react';
import { CartContext } from '../components/CartProvider';

const useCart = () => {
  const { cartItems, cartTotal, cartCount, addToCart, removeFromCart, updateQuantity, clearCart } = useContext(CartContext);

  return {
    cartItems,
    cartTotal,
    cartCount,
    addToCart,
    removeFromCart,
    updateQuantity,
    clearCart,
  };
};

export default useCart;