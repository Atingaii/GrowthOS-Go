import React from 'react';
import { Ticket, Copy, Check } from 'lucide-react';
import { mockCoupons } from '../../../mocks/growthOsMockData';

export const CouponsPage: React.FC = () => {
  const [copiedCode, setCopiedCode] = React.useState<string | null>(null);

  const handleCopy = (code: string) => {
    navigator.clipboard.writeText(code);
    setCopiedCode(code);
    setTimeout(() => setCopiedCode(null), 2000);
  };

  return (
    <div className="space-y-6 max-w-5xl mx-auto">
      <div>
        <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white flex items-center gap-2">
          <Ticket className="w-6 h-6 text-blue-500" /> 优惠券与权益中心
        </h1>
        <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
          管理您的平台折扣券、AI Agent 赠送额度与满减抵扣券。
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {mockCoupons.map((coupon) => (
          <div
            key={coupon.id}
            className={`p-5 rounded-2xl border flex items-center justify-between transition-all ${
              coupon.status === 'available'
                ? 'bg-gradient-to-r from-blue-500/5 to-indigo-500/5 border-blue-500/30'
                : 'bg-slate-100 dark:bg-slate-900 border-slate-200 dark:border-slate-800 opacity-60'
            }`}
          >
            <div className="space-y-1">
              <div className="text-[10px] font-mono font-bold uppercase text-blue-600 dark:text-blue-400">
                {coupon.category}
              </div>
              <div className="text-2xl font-extrabold text-slate-900 dark:text-white font-mono">
                {coupon.discount}
              </div>
              <div className="text-xs font-bold text-slate-800 dark:text-slate-200">{coupon.title}</div>
              <div className="text-[11px] text-slate-400">
                满 {coupon.minSpend} 可用 • 有效期至 {coupon.expiryDate}
              </div>
            </div>

            <div className="flex flex-col items-end gap-2">
              <span className={`text-[10px] font-bold px-2 py-0.5 rounded-full ${
                coupon.status === 'available' ? 'bg-emerald-100 dark:bg-emerald-950 text-emerald-600' : 'bg-slate-200 dark:bg-slate-800 text-slate-500'
              }`}>
                {coupon.status === 'available' ? '可使用' : '已使用'}
              </span>

              {coupon.status === 'available' && (
                <button
                  onClick={() => handleCopy(coupon.code)}
                  className="px-3 py-1.5 rounded-xl bg-blue-600 text-white font-bold text-xs flex items-center gap-1 hover:bg-blue-500 shadow-sm"
                >
                  {copiedCode === coupon.code ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                  {copiedCode === coupon.code ? '已复制' : coupon.code}
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
