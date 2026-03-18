import { Navigation } from '../components/Navigation';
import TSPDisplay from '../components/TSPDisplay';

const HomePage = () => {
  return (
    <div className="p-8">
      <Navigation />
      <h1 className="text-4xl font-bold mb-6">Welcome to the Home Page</h1>
      <TSPDisplay />
    </div>
  );
};

export default HomePage;