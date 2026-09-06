// Ghost AI & Sprite Animation
class Ghost {
    constructor(type, color, startTile, scatterTile) {
        this.type = type;
        this.color = color;
        this.startTile = startTile;
        this.scatterTile = scatterTile;
        this.tileSize = 16;
        this.speed = 1.75;
        this.frightenedSpeed = 1.0;
        this.eatenSpeed = 3.0;
        this.reset();
    }

    reset() {
        this.x = this.startTile.col * this.tileSize;
        this.y = this.startTile.row * this.tileSize;
        this.dir = 'UP';
        this.mode = 'SCATTER';
        this.frightenedTimer = 0;
        this.inHouse = this.type !== 'blinky';
        this.houseExitTimer = this.getDelay();
    }

    getDelay() {
        if (this.type === 'pinky') return 60;
        if (this.type === 'inky') return 180;
        if (this.type === 'clyde') return 300;
        return 0;
    }

    getTile() {
        return {
            col: Math.floor((this.x + 8) / this.tileSize),
            row: Math.floor((this.y + 8) / this.tileSize)
        };
    }

    getTarget(pacman, blinky) {
        const pTile = pacman.getTile();

        if (this.mode === 'EATEN') {
            return { col: 14, row: 14 };
        }
        if (this.mode === 'SCATTER') {
            return this.scatterTile;
        }
        if (this.mode === 'FRIGHTENED') {
            return { col: Math.floor(Math.random() * 28), row: Math.floor(Math.random() * 36) };
        }

        if (this.type === 'blinky') {
            return { col: pTile.col, row: pTile.row };
        }
        if (this.type === 'pinky') {
            let oc = 0, or = 0;
            if (pacman.dir === 'UP') { or = -4; oc = -4; }
            if (pacman.dir === 'DOWN') or = 4;
            if (pacman.dir === 'LEFT') oc = -4;
            if (pacman.dir === 'RIGHT') oc = 4;
            return { col: pTile.col + oc, row: pTile.row + or };
        }
        if (this.type === 'inky') {
            let pc = pTile.col, pr = pTile.row;
            if (pacman.dir === 'UP') { pr -= 2; pc -= 2; }
            if (pacman.dir === 'DOWN') pr += 2;
            if (pacman.dir === 'LEFT') pc -= 2;
            if (pacman.dir === 'RIGHT') pc += 2;

            const bTile = blinky.getTile();
            return { col: pc + (pc - bTile.col), row: pr + (pr - bTile.row) };
        }
        if (this.type === 'clyde') {
            const mt = this.getTile();
            const dist = Math.hypot(mt.col - pTile.col, mt.row - pTile.row);
            return dist >= 8 ? pTile : this.scatterTile;
        }

        return this.scatterTile;
    }

    getOpposite(dir) {
        if (dir === 'UP') return 'DOWN';
        if (dir === 'DOWN') return 'UP';
        if (dir === 'LEFT') return 'RIGHT';
        if (dir === 'RIGHT') return 'LEFT';
        return 'UP';
    }

    update(maze, pacman, blinky, tick) {
        if (this.inHouse) {
            if (this.houseExitTimer > 0) {
                this.houseExitTimer--;
                this.y += Math.sin(tick * 0.15) * 0.4;
                return;
            }
            this.x = 13.5 * this.tileSize;
            this.y -= 1.0;
            if (this.y <= 14 * this.tileSize) {
                this.inHouse = false;
                this.dir = 'LEFT';
            }
            return;
        }

        if (this.mode === 'FRIGHTENED') {
            this.frightenedTimer--;
            if (this.frightenedTimer <= 0) {
                this.mode = 'CHASE';
            }
        }

        if (this.mode === 'EATEN') {
            const tile = this.getTile();
            if ((tile.col === 13 || tile.col === 14) && (tile.row === 14 || tile.row === 15)) {
                this.mode = 'CHASE';
                this.dir = 'UP';
            }
        }

        let curSpeed = this.speed;
        if (this.mode === 'FRIGHTENED') curSpeed = this.frightenedSpeed;
        if (this.mode === 'EATEN') curSpeed = this.eatenSpeed;
        if (maze.isTunnel(this.getTile().col, this.getTile().row)) curSpeed *= 0.6;

        const alignedX = Math.abs(this.x - Math.round(this.x / this.tileSize) * this.tileSize) < curSpeed;
        const alignedY = Math.abs(this.y - Math.round(this.y / this.tileSize) * this.tileSize) < curSpeed;

        if (alignedX && alignedY) {
            this.x = Math.round(this.x / this.tileSize) * this.tileSize;
            this.y = Math.round(this.y / this.tileSize) * this.tileSize;

            const myTile = this.getTile();
            const target = this.getTarget(pacman, blinky);
            const opp = this.getOpposite(this.dir);

            const dirs = ['UP', 'LEFT', 'DOWN', 'RIGHT'];
            let bestDir = this.dir;
            let minDist = Infinity;

            for (const d of dirs) {
                if (d === opp && this.mode !== 'FRIGHTENED') continue;

                let nc = myTile.col;
                let nr = myTile.row;
                if (d === 'UP') nr--;
                if (d === 'DOWN') nr++;
                if (d === 'LEFT') nc--;
                if (d === 'RIGHT') nc++;

                const isDoor = maze.isGhostDoor(nc, nr);
                if (maze.isWall(nc, nr) || (isDoor && this.mode !== 'EATEN')) {
                    continue;
                }

                const dist = Math.hypot(nc - target.col, nr - target.row);
                if (dist < minDist) {
                    minDist = dist;
                    bestDir = d;
                }
            }
            this.dir = bestDir;
        }

        if (this.dir === 'LEFT') this.x -= curSpeed;
        if (this.dir === 'RIGHT') this.x += curSpeed;
        if (this.dir === 'UP') this.y -= curSpeed;
        if (this.dir === 'DOWN') this.y += curSpeed;

        if (this.x < -8) {
            this.x = 28 * this.tileSize;
        } else if (this.x > 28 * this.tileSize) {
            this.x = -8;
        }
    }

    frighten(duration = 450) {
        if (this.mode !== 'EATEN') {
            this.mode = 'FRIGHTENED';
            this.frightenedTimer = duration;
            this.dir = this.getOpposite(this.dir);
        }
    }

    eat() {
        this.mode = 'EATEN';
        this.frightenedTimer = 0;
    }

    draw(ctx, tick) {
        const x = this.x;
        const y = this.y;

        ctx.save();

        if (this.mode === 'EATEN') {
            this.drawEyes(ctx, x + 8, y + 8);
            ctx.restore();
            return;
        }

        let bodyColor = this.color;
        if (this.mode === 'FRIGHTENED') {
            const isFlashing = this.frightenedTimer < 120 && Math.floor(tick / 10) % 2 === 0;
            bodyColor = isFlashing ? '#FFFFFF' : '#2121FF';
        }

        ctx.fillStyle = bodyColor;
        ctx.beginPath();
        ctx.arc(x + 8, y + 6, 7, Math.PI, 0, false);
        ctx.lineTo(x + 15, y + 14);

        const wave = Math.floor(tick / 8) % 2 === 0;
        if (wave) {
            ctx.lineTo(x + 13, y + 12);
            ctx.lineTo(x + 10, y + 15);
            ctx.lineTo(x + 8, y + 12);
            ctx.lineTo(x + 5, y + 15);
            ctx.lineTo(x + 3, y + 12);
            ctx.lineTo(x + 1, y + 14);
        } else {
            ctx.lineTo(x + 12, y + 15);
            ctx.lineTo(x + 10, y + 13);
            ctx.lineTo(x + 8, y + 15);
            ctx.lineTo(x + 6, y + 13);
            ctx.lineTo(x + 4, y + 15);
            ctx.lineTo(x + 1, y + 13);
        }
        ctx.lineTo(x + 1, y + 6);
        ctx.fill();

        if (this.mode === 'FRIGHTENED') {
            ctx.fillStyle = '#FFAA00';
            ctx.fillRect(x + 4, y + 5, 2, 2);
            ctx.fillRect(x + 10, y + 5, 2, 2);

            ctx.strokeStyle = '#FFAA00';
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(x + 4, y + 11);
            ctx.lineTo(x + 6, y + 9);
            ctx.lineTo(x + 8, y + 11);
            ctx.lineTo(x + 10, y + 9);
            ctx.lineTo(x + 12, y + 11);
            ctx.stroke();
        } else {
            this.drawEyes(ctx, x + 8, y + 8);
        }

        ctx.restore();
    }

    drawEyes(ctx, cx, cy) {
        let px = 0, py = 0;
        if (this.dir === 'LEFT') px = -2;
        if (this.dir === 'RIGHT') px = 2;
        if (this.dir === 'UP') py = -2;
        if (this.dir === 'DOWN') py = 2;

        ctx.fillStyle = '#FFFFFF';
        ctx.beginPath();
        ctx.arc(cx - 3.5, cy - 2, 3.2, 0, Math.PI * 2);
        ctx.arc(cx + 3.5, cy - 2, 3.2, 0, Math.PI * 2);
        ctx.fill();

        ctx.fillStyle = '#0000FF';
        ctx.beginPath();
        ctx.arc(cx - 3.5 + px, cy - 2 + py, 1.8, 0, Math.PI * 2);
        ctx.arc(cx + 3.5 + px, cy - 2 + py, 1.8, 0, Math.PI * 2);
        ctx.fill();
    }
}
