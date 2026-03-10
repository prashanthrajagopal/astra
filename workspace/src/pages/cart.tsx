import { useCart } from '../context/CartContext';
import CartItem from '../components/CartItem';

const Cart = () => {
  const { cartItems, cartCount } = useCart();

  return (
    <div className="max-w-md mx-auto p-4">
      <h1 className="text-3xl font-bold">Cart</h1>
      <ul className="flex flex-wrap justify-center gap-4">
        {cartItems.map((item) => (
          <CartItem key={item.product.id} item={item} />
        ))}
      </ul>
      <p className="text-lg font-bold">
        You have {cartCount} items in your cart. Total: ${cartTotal()}
      </p>
      <button
        className="bg-indigo-500 p-2 rounded"
        onClick={() => {
          // Clear cart logic here
        }}
      >
        Clear Cart
      </button>
      <button
        className="bg-indigo-500 p-2 rounded"
        onClick={() => {
          // Proceed to checkout logic here
        }}
      >
        Proceed to Checkout
      </button>
    </div>
  );
};

export default Cart;