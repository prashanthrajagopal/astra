import { useAppRouter } from 'next/app';
import { validationStatus } from '../components/ValidatePhase1Button';

function ValidatePhase1Page() {
  const router = useAppRouter();

  if (!validationStatus.isValid) {
    return (
      <div className="bg-red-500 py-4 px-4 text-white font-bold">
        {validationStatus.message}
      </div>
    );
  }

  return (
    <div className="bg-green-500 py-4 px-4 text-white font-bold">
      Validation Phase 1 is complete!
    </div>
  );
}

export default ValidatePhase1Page;