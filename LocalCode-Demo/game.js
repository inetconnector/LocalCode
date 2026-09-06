// Main Pac-Man Arcade Engine (Namco/Konami Style 60 FPS Loop)
(function() {
    const canvas = document.getElementById('game-canvas');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');

    const audio = new PacmanAudio();
    const maze = new PacmanMaze();
    const pacman = new Pacman();

    const ghosts = [
        new Ghost('blinky', '#FF0000', { col: 13.5, row: 14 }, { col: 25, row: 0 }),
        new Ghost('pinky', '#FFB8FF', { col: 13.5, row: 17 }, { col: 2, row: 0 }),
        new Ghost('inky', '#00FFFF', { col: 11.5, row: 17 }, { col: 27, row: 35 }),
        new Ghost('clyde', '#FFB852', { col: 15.5, row: 17 }, { col: 0, row: 35 })
    ];

    let score = 0;
    let highscore = parseInt(localStorage.getItem('pacman_highscore') || '10000', 10);
    let lives = 3;
    let level = 1;
    let gameState = 'READY'; // 'READY', 'PLAYING', 'PAUSED', 'GHOST_EATEN_PAUSE', 'PACMAN_DYING', 'GAME_OVER', 'VICTORY'
    let stateTimer = 120;
    let tick = 0;
    let ghostsEatenInStreak = 0;
    let eatenScorePopup = null; // { x, y, score, timer }
    let fruit = null; // { col: 14, row: 20, type: 'cherry', score: 100, timer: 600 }

    // Wave Mode Timers for Ghosts (Scatter / Chase cycle)
    let waveTimer = 0;
    const waveSchedule = [
        { mode: 'SCATTER', dur: 420 },
        { mode: 'CHASE', dur: 1200 },
        { mode: 'SCATTER', dur: 420 },
        { mode: 'CHASE', dur: 1200 },
        { mode: 'SCATTER', dur: 300 },
        { mode: 'CHASE', dur: Infinity }
    ];
    let waveIndex = 0;

    function initGame() {
        score = 0;
        lives = 3;
        level = 1;
        maze.reset();
        resetPositions();
        gameState = 'READY';
        stateTimer = 140;
        audio.playStartIntro();
        updateHUD();
    }

    function resetPositions() {
        pacman.reset();
        ghosts.forEach(g => g.reset());
        ghostsEatenInStreak = 0;
        waveIndex = 0;
        waveTimer = 0;
    }

    function updateHUD() {
        // High score persistence
        if (score > highscore) {
            highscore = score;
            localStorage.setItem('pacman_highscore', highscore.toString());
        }
    }

    function checkCollisions() {
        const pTile = pacman.getTile();

        // 1. Eat Pellet / Energizer
        const points = maze.eat(pTile.col, pTile.row);
        if (points > 0) {
            score += points;
            updateHUD();

            if (points === 10) {
                audio.playChomp();
            } else if (points === 50) {
                audio.playEnergizer();
                ghostsEatenInStreak = 0;
                ghosts.forEach(g => g.frighten());
            }

            // Spawn Bonus Fruit at 70 and 170 pellets remaining
            if ((maze.remainingPellets === 170 || maze.remainingPellets === 70) && !fruit) {
                fruit = { col: 14, row: 20, type: 'cherry', score: 100, timer: 600 };
            }

            // Check level clear
            if (maze.remainingPellets <= 0) {
                gameState = 'VICTORY';
                stateTimer = 180;
                return;
            }
        }

        // 2. Eat Fruit
        if (fruit && pTile.col === fruit.col && pTile.row === fruit.row) {
            score += fruit.score;
            audio.playFruit();
            eatenScorePopup = { x: fruit.col * 16, y: fruit.row * 16, score: fruit.score, timer: 60 };
            fruit = null;
            updateHUD();
        }

        // 3. Ghost Collisions
        const pRadius = 7;
        for (const ghost of ghosts) {
            const dist = Math.hypot((pacman.x + 8) - (ghost.x + 8), (pacman.y + 8) - (ghost.y + 8));
            if (dist < 12) {
                if (ghost.mode === 'FRIGHTENED') {
                    // Eat Ghost!
                    ghost.eat();
                    ghostsEatenInStreak++;
                    const ghostScore = Math.pow(2, ghostsEatenInStreak) * 100;
                    score += ghostScore;
                    audio.playEatGhost();
                    eatenScorePopup = { x: ghost.x, y: ghost.y, score: ghostScore, timer: 45 };
                    gameState = 'GHOST_EATEN_PAUSE';
                    stateTimer = 35;
                    updateHUD();
                    break;
                } else if (ghost.mode === 'CHASE' || ghost.mode === 'SCATTER') {
                    // Pac-Man Dies
                    gameState = 'PACMAN_DYING';
                    pacman.isDying = true;
                    pacman.deathProgress = 0;
                    stateTimer = 90;
                    audio.playDeath();
                    break;
                }
            }
        }
    }

    function update() {
        tick++;

        if (fruit) {
            fruit.timer--;
            if (fruit.timer <= 0) fruit = null;
        }

        if (eatenScorePopup) {
            eatenScorePopup.timer--;
            if (eatenScorePopup.timer <= 0) eatenScorePopup = null;
        }

        if (gameState === 'READY') {
            stateTimer--;
            if (stateTimer <= 0) {
                gameState = 'PLAYING';
            }
            return;
        }

        if (gameState === 'GHOST_EATEN_PAUSE') {
            stateTimer--;
            if (stateTimer <= 0) {
                gameState = 'PLAYING';
            }
            return;
        }

        if (gameState === 'PACMAN_DYING') {
            pacman.update(maze);
            stateTimer--;
            if (stateTimer <= 0) {
                lives--;
                if (lives > 0) {
                    resetPositions();
                    gameState = 'READY';
                    stateTimer = 90;
                } else {
                    gameState = 'GAME_OVER';
                    stateTimer = 240;
                }
            }
            return;
        }

        if (gameState === 'VICTORY') {
            stateTimer--;
            if (stateTimer <= 0) {
                level++;
                maze.reset();
                resetPositions();
                gameState = 'READY';
                stateTimer = 100;
                audio.playStartIntro();
            }
            return;
        }

        if (gameState === 'GAME_OVER') {
            stateTimer--;
            return;
        }

        if (gameState === 'PLAYING') {
            // Update Ghost Wave Timers
            waveTimer++;
            const currentWave = waveSchedule[waveIndex];
            if (currentWave && waveTimer >= currentWave.dur) {
                waveTimer = 0;
                waveIndex++;
                const nextMode = waveSchedule[waveIndex] ? waveSchedule[waveIndex].mode : 'CHASE';
                ghosts.forEach(g => {
                    if (g.mode !== 'FRIGHTENED' && g.mode !== 'EATEN') {
                        g.mode = nextMode;
                    }
                });
            }

            // Update entities
            pacman.update(maze);
            const blinky = ghosts[0];
            ghosts.forEach(g => g.update(maze, pacman, blinky, tick));

            checkCollisions();
        }
    }

    function draw() {
        ctx.fillStyle = '#000000';
        ctx.fillRect(0, 0, canvas.width, canvas.height);

        // Header: Scores
        ctx.fillStyle = '#FFFFFF';
        ctx.font = '12px "Press Start 2P", monospace';
        ctx.fillText('1UP', 30, 20);
        ctx.fillText('HIGH SCORE', 160, 20);
        ctx.fillText('2UP', 370, 20);

        ctx.fillStyle = '#FFFF00';
        ctx.fillText(score.toString().padStart(2, '0'), 30, 36);
        ctx.fillStyle = '#FFFFFF';
        ctx.fillText(highscore.toString().padStart(2, '0'), 180, 36);
        ctx.fillText('00', 370, 36);

        // Maze
        const flashMaze = gameState === 'VICTORY' && (Math.floor(tick / 10) % 2 === 0);
        maze.draw(ctx, flashMaze, tick);

        // Bonus Fruit
        if (fruit) {
            ctx.fillStyle = '#FF0000';
            ctx.beginPath();
            ctx.arc(fruit.col * 16 + 8, fruit.row * 16 + 8, 6, 0, Math.PI * 2);
            ctx.fill();
            // Leaf
            ctx.fillStyle = '#00FF00';
            ctx.fillRect(fruit.col * 16 + 7, fruit.row * 16 + 1, 3, 3);
        }

        // Fruit / Ghost Eaten Score Popup
        if (eatenScorePopup) {
            ctx.fillStyle = '#00FFFF';
            ctx.font = '9px "Press Start 2P", monospace';
            ctx.fillText(eatenScorePopup.score.toString(), eatenScorePopup.x, eatenScorePopup.y + 8);
        }

        // Pacman & Ghosts
        pacman.draw(ctx);
        ghosts.forEach(g => g.draw(ctx, tick));

        // State Banners (READY / GAME OVER)
        if (gameState === 'READY') {
            ctx.fillStyle = '#FFFF00';
            ctx.font = '14px "Press Start 2P", monospace';
            ctx.fillText('READY!', 178, 324);
        } else if (gameState === 'PAUSED') {
            ctx.fillStyle = '#00FFFF';
            ctx.font = '14px "Press Start 2P", monospace';
            ctx.fillText('PAUSED', 174, 324);
        } else if (gameState === 'GAME_OVER') {
            ctx.fillStyle = '#FF0000';
            ctx.font = '14px "Press Start 2P", monospace';
            ctx.fillText('GAME  OVER', 148, 324);
        }

        // Footer: Lives & Level Fruit Icons
        for (let i = 0; i < lives - 1; i++) {
            const lx = 24 + (i * 24);
            const ly = 556;
            ctx.fillStyle = '#FFFF00';
            ctx.beginPath();
            ctx.arc(lx, ly, 7, 0.25 * Math.PI, 1.75 * Math.PI, false);
            ctx.lineTo(lx, ly);
            ctx.fill();
        }

        // Level fruit icon
        ctx.fillStyle = '#FF0000';
        ctx.beginPath();
        ctx.arc(420, 556, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = '#00FF00';
        ctx.fillRect(419, 549, 3, 3);
    }

    function loop() {
        update();
        draw();
        requestAnimationFrame(loop);
    }

    // Input Listeners
    window.addEventListener('keydown', (e) => {
        audio.init();

        switch (e.key) {
            case 'ArrowUp':
            case 'w':
            case 'W':
                pacman.setNextDirection('UP');
                e.preventDefault();
                break;
            case 'ArrowDown':
            case 's':
            case 'S':
                pacman.setNextDirection('DOWN');
                e.preventDefault();
                break;
            case 'ArrowLeft':
            case 'a':
            case 'A':
                pacman.setNextDirection('LEFT');
                e.preventDefault();
                break;
            case 'ArrowRight':
            case 'd':
            case 'D':
                pacman.setNextDirection('RIGHT');
                e.preventDefault();
                break;
            case ' ':
                if (gameState === 'PLAYING') {
                    gameState = 'PAUSED';
                } else if (gameState === 'PAUSED') {
                    gameState = 'PLAYING';
                }
                e.preventDefault();
                break;
            case 'Enter':
            case 'r':
            case 'R':
                if (gameState === 'GAME_OVER') {
                    initGame();
                }
                break;
        }
    });

    // Touch D-Pad & Control Buttons
    const bindBtn = (id, fn) => {
        const el = document.getElementById(id);
        if (!el) return;
        ['touchstart', 'mousedown'].forEach(evt => {
            el.addEventListener(evt, (e) => {
                e.preventDefault();
                audio.init();
                fn();
            });
        });
    };

    bindBtn('dpad-up', () => pacman.setNextDirection('UP'));
    bindBtn('dpad-down', () => pacman.setNextDirection('DOWN'));
    bindBtn('dpad-left', () => pacman.setNextDirection('LEFT'));
    bindBtn('dpad-right', () => pacman.setNextDirection('RIGHT'));

    bindBtn('btn-start', () => {
        if (gameState === 'GAME_OVER') {
            initGame();
        } else if (gameState === 'PLAYING') {
            gameState = 'PAUSED';
        } else if (gameState === 'PAUSED') {
            gameState = 'PLAYING';
        }
    });

    bindBtn('btn-audio', () => {
        const isMuted = audio.toggleMute();
        const btn = document.getElementById('btn-audio');
        if (btn) btn.textContent = isMuted ? 'SOUND: OFF' : 'SOUND: ON';
    });

    bindBtn('btn-reset', () => {
        initGame();
    });

    // Start Engine
    initGame();
    requestAnimationFrame(loop);
})();
