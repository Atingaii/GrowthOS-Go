import React, { useState } from 'react';
import { Trophy, Coins, Sparkles, AlertCircle, RefreshCw } from 'lucide-react';
import { mockLotteryPrizes } from '../../../mocks/growthOsMockData';
import { LotteryPrize } from '../../../types/growthos';

export const LotteryPage: React.FC = () => {
  const [spinning, setSpinning] = useState(false);
  const [result, setResult] = useState<LotteryPrize | null>(null);
  const [userPoints, setUserPoints] = useState(12450);

  const handleSpin = () => {
    if (spinning || userPoints < 100) return;
    setSpinning(true);
    setResult(null);
    setUserPoints((prev) => prev - 100);

    setTimeout(() => {
      const winner = mockLotteryPrizes[Math.floor(Math.random() * mockLotteryPrizes.length)];
      setResult(winner);
      setSpinning(false);
    }, 2000);
  };

  return (
    <div className="max-w-4xl mx-auto space-y-8">
      {/* Header */}
      <div className="text-center space-y-2">
        <div className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-amber-100 dark:bg-amber-950/60 text-amber-700 dark:text-amber-400 text-xs font-bold">
          <Trophy className="w-4 h-4" /> GrowthOS 抽奖引擎
        </div>
        <h1 className="text-3xl font-extrabold text-slate-900 dark:text-white">幸运轮盘大抽奖</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          每次抽奖消耗 <span className="font-bold text-amber-500">100 Points</span>，100% 具备策略中奖概率
        </p>
      </div>

      {/* Lottery Wheel Panel */}
      <div className="bg-white dark:bg-slate-900 rounded-3xl p-8 border border-slate-200 dark:border-slate-800 shadow-xl flex flex-col items-center">
        {/* Points Display */}
        <div className="flex items-center gap-2 mb-8 bg-slate-100 dark:bg-slate-800 px-4 py-2 rounded-2xl font-mono text-sm font-bold">
          <Coins className="w-5 h-5 text-amber-500" />
          <span>可用积分: <span className="text-blue-600 dark:text-blue-400 text-base">{userPoints}</span> PTS</span>
        </div>

        {/* Wheel Grid */}
        <div className="grid grid-cols-3 gap-3 w-full max-w-md aspect-square mb-8">
          {mockLotteryPrizes.map((prize) => (
            <div
              key={prize.id}
              className={`p-4 rounded-2xl flex flex-col items-center justify-center text-center border-2 transition-all ${
                result?.id === prize.id
                  ? 'border-amber-400 bg-amber-500/10 scale-105 shadow-lg shadow-amber-500/20'
                  : 'border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/50'
              }`}
            >
              <div className="w-8 h-8 rounded-full flex items-center justify-center mb-2 font-bold" style={{ backgroundColor: prize.color + '20', color: prize.color }}>
                🎁
              </div>
              <div className="text-xs font-bold text-slate-800 dark:text-slate-200">{prize.name}</div>
              <div className="text-[10px] text-slate-400 mt-1">{prize.amount}</div>
            </div>
          ))}
        </div>

        {/* Spin CTA Button */}
        <button
          onClick={handleSpin}
          disabled={spinning || userPoints < 100}
          className={`w-full max-w-xs py-3.5 rounded-2xl font-extrabold text-base flex items-center justify-center gap-2 shadow-lg transition-all ${
            spinning
              ? 'bg-slate-400 cursor-not-allowed text-white'
              : 'bg-gradient-to-r from-amber-500 to-orange-500 hover:from-amber-400 hover:to-orange-400 text-white shadow-amber-500/25 active:scale-95'
          }`}
        >
          {spinning ? (
            <>
              <RefreshCw className="w-5 h-5 animate-spin" />
              正在匹配抽奖策略...
            </>
          ) : (
            <>
              <Sparkles className="w-5 h-5" />
              立即抽奖 (100 PTS)
            </>
          )}
        </button>

        {/* Winner Announcement Popup */}
        {result && (
          <div className="mt-6 p-4 rounded-2xl bg-emerald-50 dark:bg-emerald-950/60 border border-emerald-500/30 text-emerald-800 dark:text-emerald-300 flex items-center gap-3 animate-bounce">
            <Trophy className="w-6 h-6 text-emerald-500 shrink-0" />
            <div>
              <div className="text-xs font-bold uppercase">恭喜中奖！</div>
              <div className="text-sm font-extrabold">获得了 {result.name} ({result.amount})</div>
            </div>
          </div>
        )}
      </div>

      {/* Rules Notice */}
      <div className="bg-slate-100 dark:bg-slate-800/50 rounded-2xl p-4 flex items-start gap-3 text-xs text-slate-500 dark:text-slate-400">
        <AlertCircle className="w-4 h-4 text-blue-500 shrink-0 mt-0.5" />
        <div>
          <span className="font-bold text-slate-700 dark:text-slate-300">抽奖引擎说明:</span> 抽奖规则由 GrowthOS-Go 后端 <code className="font-mono text-blue-500">lottery-service</code> 统一概率管控，防刷与限流由 Redis 分布式锁保护。
        </div>
      </div>
    </div>
  );
};
