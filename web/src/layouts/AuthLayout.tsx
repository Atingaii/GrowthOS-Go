import React from 'react';
import { Link, Outlet } from 'react-router';
import { GrowthOSLogo } from '../components/common/GrowthOSGraphics';

export const AuthLayout: React.FC = () => {
  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col justify-center items-center p-4 relative overflow-hidden">
      {/* Background Graphic Grid */}
      <div className="absolute inset-0 bg-grid-pattern opacity-20 pointer-events-none" />
      <div className="absolute top-1/4 left-1/2 -translate-x-1/2 w-96 h-96 bg-blue-600/20 rounded-full blur-3xl pointer-events-none" />

      <div className="relative z-10 w-full max-w-md bg-slate-900 border border-slate-800 rounded-3xl p-8 shadow-2xl">
        <div className="flex justify-center mb-8">
          <Link to="/home">
            <GrowthOSLogo />
          </Link>
        </div>
        <Outlet />
      </div>
    </div>
  );
};
