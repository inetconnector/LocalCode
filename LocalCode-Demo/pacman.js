// Pac-Man Player with Cornering
class Pacman {
    constructor() {
        this.tileSize = 16;
        this.startX = 13.5 * 16;
        this.startY = 26 * 16;
        this.speed = 2.0;
        this.reset();
    }

    reset() {
        this.x = this.startX;
        this.y = this.startY;
        this.dir = 'LEFT';
        this.nextDir = 'LEFT';
        this.mouthAngle = 0.2;
        this.mouthSpeed = 0.04;
        this.mouthOpening = true;
        this.isDying = false;
        this.deathProgress = 0;
    }

    setNextDirection(dir) {
        this.nextDir = dir;
    }

    getTile() {
        return {
            col: Math.floor((this.x + 8) / this.tileSize),
            row: Math.floor((this.y + 8) / this.tileSize)
        };
    }

    canMove(dir, maze) {
        const curCol = Math.round(this.x / this.tileSize);
        const curRow = Math.round(this.y / this.tileSize);
        let nextCol = curCol;
        let nextRow = curRow;

        if (dir === 'UP') nextRow--;
        if (dir === 'DOWN') nextRow++;
        if (dir === 'LEFT') nextCol--;
        if (dir === 'RIGHT') nextCol++;

        return !maze.isWall(nextCol, nextRow) && !maze.isGhostDoor(nextCol, nextRow);
    }

    update(maze) {
        if (this.isDying) {
            this.deathProgress += 0.025;
            return;
        }

        const isOpposite = (
            (this.dir === 'LEFT' && this.nextDir === 'RIGHT') ||
            (this.dir === 'RIGHT' && this.nextDir === 'LEFT') ||
            (this.dir === 'UP' && this.nextDir === 'DOWN') ||
            (this.dir === 'DOWN' && this.nextDir === 'UP')
        );

        if (isOpposite) {
            this.dir = this.nextDir;
        }

        const alignedX = Math.abs(this.x - Math.round(this.x / this.tileSize) * this.tileSize) < this.speed;
        const alignedY = Math.abs(this.y - Math.round(this.y / this.tileSize) * this.tileSize) < this.speed;

        if (alignedX && alignedY && this.nextDir && this.canMove(this.nextDir, maze)) {
            this.x = Math.round(this.x / this.tileSize) * this.tileSize;
            this.y = Math.round(this.y / this.tileSize) * this.tileSize;
            this.dir = this.nextDir;
        }

        let moved = false;
        if (this.dir === 'LEFT') {
            if (alignedY && (this.x > Math.round(this.x / this.tileSize) * this.tileSize || this.canMove('LEFT', maze))) {
                this.y = Math.round(this.y / this.tileSize) * this.tileSize;
                this.x -= this.speed;
                moved = true;
            }
        } else if (this.dir === 'RIGHT') {
            if (alignedY && (this.x < Math.round(this.x / this.tileSize) * this.tileSize || this.canMove('RIGHT', maze))) {
                this.y = Math.round(this.y / this.tileSize) * this.tileSize;
                this.x += this.speed;
                moved = true;
            }
        } else if (this.dir === 'UP') {
            if (alignedX && (this.y > Math.round(this.y / this.tileSize) * this.tileSize || this.canMove('UP', maze))) {
                this.x = Math.round(this.x / this.tileSize) * this.tileSize;
                this.y -= this.speed;
                moved = true;
            }
        } else if (this.dir === 'DOWN') {
            if (alignedX && (this.y < Math.round(this.y / this.tileSize) * this.tileSize || this.canMove('DOWN', maze))) {
                this.x = Math.round(this.x / this.tileSize) * this.tileSize;
                this.y += this.speed;
                moved = true;
            }
        }

        // Tunnel warp
        if (this.x < -8) {
            this.x = 28 * this.tileSize;
        } else if (this.x > 28 * this.tileSize) {
            this.x = -8;
        }

        if (moved) {
            if (this.mouthOpening) {
                this.mouthAngle += this.mouthSpeed;
                if (this.mouthAngle >= 0.45) this.mouthOpening = false;
            } else {
                this.mouthAngle -= this.mouthSpeed;
                if (this.mouthAngle <= 0.02) this.mouthOpening = true;
            }
        }
    }

    draw(ctx) {
        ctx.save();
        ctx.translate(this.x + 8, this.y + 8);

        if (this.isDying) {
            const progress = Math.min(1.0, this.deathProgress);
            const startAngle = progress * Math.PI;
            const endAngle = (2 - progress) * Math.PI;

            ctx.fillStyle = '#FFFF00';
            ctx.beginPath();
            ctx.arc(0, 0, 7.5, startAngle, endAngle, false);
            ctx.lineTo(0, 0);
            ctx.fill();
            ctx.restore();
            return;
        }

        let rotation = 0;
        if (this.dir === 'RIGHT') rotation = 0;
        if (this.dir === 'DOWN') rotation = Math.PI / 2;
        if (this.dir === 'LEFT') rotation = Math.PI;
        if (this.dir === 'UP') rotation = -Math.PI / 2;

        ctx.rotate(rotation);

        ctx.fillStyle = '#FFFF00';
        ctx.beginPath();
        ctx.arc(0, 0, 7.5, this.mouthAngle * Math.PI, (2 - this.mouthAngle) * Math.PI, false);
        ctx.lineTo(0, 0);
        ctx.fill();

        ctx.restore();
    }
}
