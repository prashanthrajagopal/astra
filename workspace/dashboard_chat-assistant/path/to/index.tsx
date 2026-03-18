import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import GreatestOfThree from '../GreatestOfThree';

const router = createBrowserRouter([
  {
    path: '/',
    element: <GreatestOfThree />,
  },
]);

function App() {
  return <RouterProvider router={router} />;
}

export default App;