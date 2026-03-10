import { Product } from '../types';

const CartItem = ({ item }: { item: CartItem }) => {
  return (
    <div className="bg-white p-4 shadow-md rounded">
      <img src={item.product.image} alt={item.product.name} className="w-full h-64 object-cover" />
      <h2 className="text-lg font-bold">{item.product.name}</h2>
      <p className="text-gray-500">{item.product.description}</p>
      <p className="text-indigo-500 font-bold">${item.product.price}</p>
      <p className="text-gray-500">{item.quantity}</p>
      <button
        className="bg-gray-200 p-2 rounded"
        onClick={() => {
          // Update quantity logic here
        }}
      >
        Update Quantity
      </button>
      <button
        className="bg-gray-200 p-2 rounded"
        onClick={() => {
          // Remove from cart logic here
        }}
      >
        Remove from Cart
      </button>
    </div>
  );
};

export default CartItem;