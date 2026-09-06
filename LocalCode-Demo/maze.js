// Authentic 1980 Arcade Maze
class PacmanMaze {
    constructor() {
        this.cols = 28;
        this.rows = 36;
        this.tileSize = 16;
        
        // 0: Empty/Path, 1: Wall, 2: Dot, 3: Energizer, 4: Ghost Door, 5: Ghost House
        this.rawGrid = [
            "0000000000000000000000000000", // 0
            "0000000000000000000000000000", // 1
            "0000000000000000000000000000", // 2
            "1111111111111111111111111111", // 3 Top border
            "1222222222222112222222222221", // 4
            "1211112111112112111112111121", // 5
            "1311112111112112111112111131", // 6 Energizer
            "1211112111112112111112111121", // 7
            "1222222222222222222222222221", // 8
            "1211112112111111112112111121", // 9
            "1211112112111111112112111121", // 10
            "1222222112222112222112222221", // 11
            "1111112111110110111112111111", // 12
            "0000012111110110111112100000", // 13
            "0000012110000000000112100000", // 14
            "0000012110111441110112100000", // 15 Ghost Door
            "1111112110155555510112111111", // 16 Ghost House
            "0000002000155555510002000000", // 17 Tunnel
            "1111112110155555510112111111", // 18
            "0000012110111111110112100000", // 19
            "0000012110000000000112100000", // 20
            "0000012110111111110112100000", // 21
            "1111112110111111110112111111", // 22
            "1222222222222112222222222221", // 23
            "1211112111112112111112111121", // 24
            "1211112111112112111112111121", // 25
            "1322112222222002222222112231", // 26 Energizer
            "1112112112111111112112112111", // 27
            "1112112112111111112112112111", // 28
            "1222222112222112222112222221", // 29
            "1211111111112112111111111121", // 30
            "1211111111112112111111111121", // 31
            "1222222222222222222222222221", // 32
            "1111111111111111111111111111", // 33 Bottom border
            "0000000000000000000000000000", // 34
            "0000000000000000000000000000"  // 35
        ];

        this.grid = [];
        this.totalPellets = 0;
        this.remainingPellets = 0;
        this.reset();
    }

    reset() {
        this.grid = [];
        this.totalPellets = 0;
        for (let r = 0; r < this.rows; r++) {
            const rowArr = [];
            for (let c = 0; c < this.cols; c++) {
                const char = this.rawGrid[r] ? this.rawGrid[r][c] : '0';
                const cell = parseInt(char, 10) || 0;
                rowArr.push(cell);
                if (cell === 2 || cell === 3) {
                    this.totalPellets++;
                }
            }
            this.grid.push(rowArr);
        }
        this.remainingPellets = this.totalPellets;
    }

    isWall(col, row) {
        if (row < 0 || row >= this.rows) return true;
        if (col < 0 || col >= this.cols) {
            return row !== 17; // Tunnel open
        }
        return this.grid[row][col] === 1;
    }

    isGhostDoor(col, row) {
        if (row < 0 || row >= this.rows || col < 0 || col >= this.cols) return false;
        return this.grid[row][col] === 4;
    }

    isTunnel(col, row) {
        return row === 17 && (col <= 5 || col >= 22);
    }

    eat(col, row) {
        if (row < 0 || row >= this.rows || col < 0 || col >= this.cols) return 0;
        const cell = this.grid[row][col];
        if (cell === 2) {
            this.grid[row][col] = 0;
            this.remainingPellets--;
            return 10;
        } else if (cell === 3) {
            this.grid[row][col] = 0;
            this.remainingPellets--;
            return 50;
        }
        return 0;
    }

    draw(ctx, flashWhite = false, tick = 0) {
        const ts = this.tileSize;
        const wallStroke = flashWhite ? '#FFFFFF' : '#2121FF';
        const doorColor = '#FFB8FF';
        const dotColor = '#FFB897';

        for (let r = 0; r < this.rows; r++) {
            for (let c = 0; c < this.cols; c++) {
                const cell = this.grid[r][c];
                const x = c * ts;
                const y = r * ts;

                if (cell === 1) {
                    // Solid dark blue core with bright arcade border
                    ctx.fillStyle = flashWhite ? '#FFFFFF' : '#1919A6';
                    ctx.fillRect(x, y, ts, ts);

                    // Authentic double outline styling
                    ctx.fillStyle = '#000000';
                    const top = r > 0 && this.isWall(c, r - 1);
                    const bot = r < this.rows - 1 && this.isWall(c, r + 1);
                    const left = c > 0 && this.isWall(c - 1, r);
                    const right = c < this.cols - 1 && this.isWall(c + 1, r);

                    const pad = 2;
                    let innerX = x + (left ? 0 : pad);
                    let innerY = y + (top ? 0 : pad);
                    let innerW = ts - (left ? 0 : pad) - (right ? 0 : pad);
                    let innerH = ts - (top ? 0 : pad) - (bot ? 0 : pad);

                    ctx.fillRect(innerX + 1, innerY + 1, Math.max(0, innerW - 2), Math.max(0, innerH - 2));
                } else if (cell === 4) {
                    ctx.fillStyle = doorColor;
                    ctx.fillRect(x, y + 6, ts, 4);
                } else if (cell === 2) {
                    ctx.fillStyle = dotColor;
                    ctx.fillRect(x + 6, y + 6, 4, 4);
                } else if (cell === 3) {
                    if (Math.floor(tick / 12) % 2 === 0) {
                        ctx.fillStyle = dotColor;
                        ctx.beginPath();
                        ctx.arc(x + 8, y + 8, 6.5, 0, Math.PI * 2);
                        ctx.fill();
                    }
                }
            }
        }
    }
}
