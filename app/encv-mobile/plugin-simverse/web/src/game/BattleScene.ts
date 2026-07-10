import Phaser from "phaser";
import { PHASER_EVENTS, phaserEventBus } from "./PhaserEventBus";

export interface BattleSceneData {
  enemyName: string;
  enemyLevel: number;
  enemyEmoji: string;
  playerEmoji: string;
  playerName: string;
  playerLevel: number;
  backTo: string;
}

interface BattleUnit {
  name: string;
  hp: number;
  maxHp: number;
  mp: number;
  maxMp: number;
  atk: number;
  def: number;
  level: number;
  emoji: string;
  container?: Phaser.GameObjects.Container;
  hpBar?: Phaser.GameObjects.Graphics;
  mpBar?: Phaser.GameObjects.Graphics;
}

export class BattleScene extends Phaser.Scene {
  private battleData!: BattleSceneData;
  private player!: BattleUnit;
  private enemy!: BattleUnit;
  private battleLog: string[] = [];
  private isPlayerTurn = true;
  private battleEnded = false;
  private logText!: Phaser.GameObjects.Text;
  private actionButtons: Phaser.GameObjects.Container[] = [];

  constructor() {
    super("BattleScene");
  }

  init(data: BattleSceneData): void {
    this.battleData = data;
  }

  create(): void {
    this.cameras.main.setBackgroundColor("#0f0f1a");

    this.createBattleBackground();
    this.createPlayer();
    this.createEnemy();
    this.createBattleUI();
    this.createActionButtons();
    this.createBackButton();

    this.addLog(`遭遇了 ${this.battleData.enemyName}！`);
    this.addLog("战斗开始！");

    phaserEventBus.emit(PHASER_EVENTS.BATTLE_START, this.battleData);
  }

  private createBattleBackground(): void {
    const { width, height } = this.scale;

    const bg = this.add.graphics();
    const gradientColors: [number, number, number][] = [
      [20, 10, 40],
      [40, 20, 60],
      [30, 15, 50],
    ];

    for (let y = 0; y < height; y += 2) {
      const t = y / height;
      const colorIndex = Math.min(Math.floor(t * (gradientColors.length - 1)), gradientColors.length - 2);
      const localT = (t * (gradientColors.length - 1)) % 1;
      const c1 = gradientColors[colorIndex];
      const c2 = gradientColors[colorIndex + 1];
      const r = Phaser.Math.Linear(c1[0], c2[0], localT);
      const g = Phaser.Math.Linear(c1[1], c2[1], localT);
      const b = Phaser.Math.Linear(c1[2], c2[2], localT);
      bg.fillStyle(Phaser.Display.Color.GetColor(r, g, b), 1);
      bg.fillRect(0, y, width, 2);
    }

    const ground = this.add.graphics();
    ground.fillStyle(0x1a1a2e, 1);
    ground.fillRect(0, height * 0.65, width, height * 0.35);

    for (let i = 0; i < 20; i++) {
      const x = Phaser.Math.Between(0, width);
      const y = Phaser.Math.Between(height * 0.65, height * 0.95);
      const size = Phaser.Math.Between(2, 6);
      const dot = this.add.circle(x, y, size, 0x2a2a4e, 0.5);
      dot.setAlpha(Phaser.Math.FloatBetween(0.3, 0.7));
    }
  }

  private createPlayer(): void {
    const { width, height } = this.scale;

    this.player = {
      name: this.battleData.playerName || "勇者",
      hp: 100,
      maxHp: 100,
      mp: 50,
      maxMp: 50,
      atk: 15,
      def: 8,
      level: this.battleData.playerLevel || 1,
      emoji: this.battleData.playerEmoji || "⚔️",
    };

    const container = this.add.container(width * 0.25, height * 0.5);

    const shadow = this.add.circle(0, 60, 50, 0x000000, 0.3);
    shadow.setScale(1, 0.3);

    const sprite = this.add
      .text(0, 0, this.player.emoji, {
        fontSize: "80px",
      })
      .setOrigin(0.5);

    const nameText = this.add
      .text(0, 70, `${this.player.name} Lv.${this.player.level}`, {
        fontSize: "16px",
        color: "#ffffff",
        fontStyle: "bold",
      })
      .setOrigin(0.5);

    const hpBarBg = this.add.graphics();
    hpBarBg.fillStyle(0x333333, 1);
    hpBarBg.fillRect(-60, 95, 120, 10);

    const hpBar = this.add.graphics();
    this.updateHpBar(hpBar, this.player.hp, this.player.maxHp, 0x22c55e);

    const mpBarBg = this.add.graphics();
    mpBarBg.fillStyle(0x333333, 1);
    mpBarBg.fillRect(-60, 110, 120, 6);

    const mpBar = this.add.graphics();
    this.updateHpBar(mpBar, this.player.mp, this.player.maxMp, 0x3b82f6, 6);

    const hpText = this.add
      .text(0, 125, `${this.player.hp}/${this.player.maxHp}`, {
        fontSize: "12px",
        color: "#ffffff",
      })
      .setOrigin(0.5);

    container.add([shadow, sprite, nameText, hpBarBg, hpBar, mpBarBg, mpBar, hpText]);

    this.player.container = container;
    this.player.hpBar = hpBar;
    this.player.mpBar = mpBar;
  }

  private createEnemy(): void {
    const { width, height } = this.scale;

    this.enemy = {
      name: this.battleData.enemyName,
      hp: 80,
      maxHp: 80,
      mp: 30,
      maxMp: 30,
      atk: 12,
      def: 5,
      level: this.battleData.enemyLevel,
      emoji: this.battleData.enemyEmoji,
    };

    const container = this.add.container(width * 0.75, height * 0.5);

    const shadow = this.add.circle(0, 60, 50, 0x000000, 0.3);
    shadow.setScale(1, 0.3);

    const sprite = this.add
      .text(0, 0, this.enemy.emoji, {
        fontSize: "80px",
      })
      .setOrigin(0.5);
    sprite.setFlipX(true);

    const nameText = this.add
      .text(0, 70, `${this.enemy.name} Lv.${this.enemy.level}`, {
        fontSize: "16px",
        color: "#f87171",
        fontStyle: "bold",
      })
      .setOrigin(0.5);

    const hpBarBg = this.add.graphics();
    hpBarBg.fillStyle(0x333333, 1);
    hpBarBg.fillRect(-60, 95, 120, 10);

    const hpBar = this.add.graphics();
    this.updateHpBar(hpBar, this.enemy.hp, this.enemy.maxHp, 0xef4444);

    const hpText = this.add
      .text(0, 115, `${this.enemy.hp}/${this.enemy.maxHp}`, {
        fontSize: "12px",
        color: "#ffffff",
      })
      .setOrigin(0.5);

    container.add([shadow, sprite, nameText, hpBarBg, hpBar, hpText]);

    this.enemy.container = container;
    this.enemy.hpBar = hpBar;
  }

  private updateHpBar(graphics: Phaser.GameObjects.Graphics, current: number, max: number, color: number, barHeight: number = 10): void {
    graphics.clear();
    graphics.fillStyle(color, 1);
    const width = 120 * (current / max);
    graphics.fillRect(-60, 0, width, barHeight);
  }

  private createBattleUI(): void {
    const { width, height } = this.scale;

    const logBg = this.add.graphics();
    logBg.fillStyle(0x000000, 0.7);
    logBg.fillRect(20, height - 140, width - 40, 100);
    logBg.lineStyle(2, 0x6366f1, 0.5);
    logBg.strokeRect(20, height - 140, width - 40, 100);

    this.logText = this.add.text(35, height - 130, "", {
      fontSize: "14px",
      color: "#ffffff",
      wordWrap: { width: width - 70 },
    });
    this.logText.setLineSpacing(4);
  }

  private createActionButtons(): void {
    const { width, height } = this.scale;

    const actions = [
      { label: "⚔️ 攻击", action: "attack", color: 0xef4444 },
      { label: "🛡️ 防御", action: "defend", color: 0x3b82f6 },
      { label: "✨ 技能", action: "skill", color: 0xa855f7 },
      { label: "🏃 逃跑", action: "flee", color: 0x6b7280 },
    ];

    const btnWidth = 100;
    const btnHeight = 50;
    const spacing = 15;
    const totalWidth = actions.length * btnWidth + (actions.length - 1) * spacing;
    const startX = (width - totalWidth) / 2 + btnWidth / 2;

    actions.forEach((action, index) => {
      const x = startX + index * (btnWidth + spacing);
      const y = height - 30;

      const container = this.add.container(x, y);

      const bg = this.add.graphics();
      bg.fillStyle(action.color, 0.8);
      bg.fillRoundedRect(-btnWidth / 2, -btnHeight / 2, btnWidth, btnHeight, 8);

      const label = this.add
        .text(0, 0, action.label, {
          fontSize: "16px",
          color: "#ffffff",
          fontStyle: "bold",
        })
        .setOrigin(0.5);

      container.add([bg, label]);
      container.setSize(btnWidth, btnHeight);
      container.setInteractive({ useHandCursor: true });

      container.on("pointerdown", () => {
        if (this.isPlayerTurn && !this.battleEnded) {
          this.handlePlayerAction(action.action);
        }
      });

      container.on("pointerover", () => {
        this.tweens.add({
          targets: bg,
          alpha: 0.6,
          duration: 100,
        });
      });

      container.on("pointerout", () => {
        this.tweens.add({
          targets: bg,
          alpha: 0.8,
          duration: 100,
        });
      });

      this.actionButtons.push(container);
    });
  }

  private createBackButton(): void {
    const btn = this.add
      .text(20, 20, "← 返回", {
        fontSize: "16px",
        color: "#ffffff",
        backgroundColor: "rgba(0,0,0,0.6)",
        padding: { x: 12, y: 6 },
      })
      .setInteractive({ useHandCursor: true });

    btn.on("pointerdown", () => {
      this.endBattle("flee");
    });
  }

  private handlePlayerAction(action: string): void {
    if (!this.isPlayerTurn || this.battleEnded) return;

    this.setButtonsEnabled(false);
    this.isPlayerTurn = false;

    switch (action) {
      case "attack":
        this.playerAttack();
        break;
      case "defend":
        this.playerDefend();
        break;
      case "skill":
        this.playerSkill();
        break;
      case "flee":
        this.playerFlee();
        break;
    }
  }

  private playerAttack(): void {
    this.addLog(`${this.player.name} 发动攻击！`);

    this.tweens.add({
      targets: this.player.container,
      x: this.player.container!.x + 80,
      duration: 200,
      yoyo: true,
      onComplete: () => {
        const damage = Math.max(1, this.player.atk - this.enemy.def + Phaser.Math.Between(-3, 3));
        this.enemy.hp = Math.max(0, this.enemy.hp - damage);
        this.updateHpBar(this.enemy.hpBar!, this.enemy.hp, this.enemy.maxHp, 0xef4444);
        this.addLog(`造成 ${damage} 点伤害！`);

        this.shakeUnit(this.enemy.container!);

        if (this.enemy.hp <= 0) {
          this.endBattle("win");
        } else {
          this.time.delayedCall(800, () => this.enemyTurn());
        }
      },
    });
  }

  private playerDefend(): void {
    this.addLog(`${this.player.name} 进入防御姿态！`);
    this.player.def += 5;

    this.tweens.add({
      targets: this.player.container,
      scale: { from: 1, to: 1.05 },
      duration: 200,
      yoyo: true,
    });

    this.time.delayedCall(800, () => this.enemyTurn());
  }

  private playerSkill(): void {
    if (this.player.mp < 10) {
      this.addLog("MP不足！");
      this.isPlayerTurn = true;
      this.setButtonsEnabled(true);
      return;
    }

    this.player.mp -= 10;
    this.updateHpBar(this.player.mpBar!, this.player.mp, this.player.maxMp, 0x3b82f6, 6);

    this.addLog(`${this.player.name} 使用了火焰术！`);

    const effect = this.add.circle(this.enemy.container!.x, this.enemy.container!.y, 10, 0xf97316, 0.8);

    this.tweens.add({
      targets: effect,
      scale: { from: 0.5, to: 3 },
      alpha: { from: 1, to: 0 },
      duration: 500,
      onComplete: () => {
        effect.destroy();
        const damage = Math.max(1, Math.floor(this.player.atk * 1.8) - this.enemy.def + Phaser.Math.Between(-2, 5));
        this.enemy.hp = Math.max(0, this.enemy.hp - damage);
        this.updateHpBar(this.enemy.hpBar!, this.enemy.hp, this.enemy.maxHp, 0xef4444);
        this.addLog(`造成 ${damage} 点火焰伤害！`);

        this.shakeUnit(this.enemy.container!);

        if (this.enemy.hp <= 0) {
          this.endBattle("win");
        } else {
          this.time.delayedCall(800, () => this.enemyTurn());
        }
      },
    });
  }

  private playerFlee(): void {
    const fleeChance = 0.6;
    if (Math.random() < fleeChance) {
      this.addLog("成功逃跑！");
      this.time.delayedCall(800, () => this.endBattle("flee"));
    } else {
      this.addLog("逃跑失败！");
      this.time.delayedCall(800, () => this.enemyTurn());
    }
  }

  private enemyTurn(): void {
    if (this.battleEnded) return;

    this.addLog(`${this.enemy.name} 发动攻击！`);

    this.tweens.add({
      targets: this.enemy.container,
      x: this.enemy.container!.x - 80,
      duration: 200,
      yoyo: true,
      onComplete: () => {
        const damage = Math.max(1, this.enemy.atk - this.player.def + Phaser.Math.Between(-2, 2));
        this.player.hp = Math.max(0, this.player.hp - damage);
        this.updateHpBar(this.player.hpBar!, this.player.hp, this.player.maxHp, 0x22c55e);
        this.addLog(`受到 ${damage} 点伤害！`);

        this.shakeUnit(this.player.container!);

        if (this.player.hp <= 0) {
          this.endBattle("lose");
        } else {
          this.player.def = Math.max(8, this.player.def - 5);
          this.isPlayerTurn = true;
          this.setButtonsEnabled(true);
        }
      },
    });
  }

  private shakeUnit(container: Phaser.GameObjects.Container): void {
    this.cameras.main.shake(200, 0.01);

    this.tweens.add({
      targets: container,
      x: container.x - 10,
      duration: 50,
      yoyo: true,
      repeat: 3,
    });
  }

  private setButtonsEnabled(enabled: boolean): void {
    this.actionButtons.forEach(btn => {
      btn.setInteractive(enabled);
      btn.setAlpha(enabled ? 1 : 0.5);
    });
  }

  private addLog(message: string): void {
    this.battleLog.push(message);
    if (this.battleLog.length > 5) {
      this.battleLog.shift();
    }
    this.logText.setText(this.battleLog.join("\n"));
  }

  private endBattle(result: "win" | "lose" | "flee"): void {
    this.battleEnded = true;
    this.setButtonsEnabled(false);

    switch (result) {
      case "win":
        this.addLog(`战斗胜利！获得经验值！`);
        this.tweens.add({
          targets: this.enemy.container,
          alpha: 0,
          scale: 0.5,
          duration: 500,
        });
        break;
      case "lose":
        this.addLog(`战斗失败...`);
        this.tweens.add({
          targets: this.player.container,
          alpha: 0,
          scale: 0.5,
          duration: 500,
        });
        break;
      case "flee":
        this.addLog(`脱离了战斗`);
        break;
    }

    phaserEventBus.emit(PHASER_EVENTS.BATTLE_END, { result, data: this.battleData });

    this.time.delayedCall(1500, () => {
      this.scene.start("WorldScene");
      phaserEventBus.emit(PHASER_EVENTS.BACK_TO_WORLD);
    });
  }
}
