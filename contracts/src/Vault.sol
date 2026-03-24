// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

interface IERC20Minimal {
    function transfer(address to, uint256 amount) external returns (bool);
}

contract Vault {
    address public owner;
    bool public paused;

    mapping(address => bool) public withdrawExecutors;
    mapping(address => bool) public routerManagers;
    mapping(address => bool) public allowedRouters;
    mapping(address => bool) public allowedTokens;
    mapping(bytes32 => bool) public processedWithdrawIds;

    uint256 private unlocked = 1;

    event WithdrawExecuted(bytes32 indexed withdrawId, address indexed token, address indexed to, uint256 amount, address operator);
    event RouterAllowedUpdated(address indexed router, bool allowed, address operator);
    event TokenAllowedUpdated(address indexed token, bool allowed, address operator);
    event WithdrawExecutorUpdated(address indexed account, bool allowed, address operator);
    event RouterManagerUpdated(address indexed account, bool allowed, address operator);
    event Paused(address operator);
    event Unpaused(address operator);
    event RescueExecuted(bytes32 indexed rescueId, address indexed token, address indexed to, uint256 amount, address operator);

    modifier onlyOwner() {
        require(msg.sender == owner, "NOT_OWNER");
        _;
    }

    modifier onlyWithdrawExecutor() {
        require(withdrawExecutors[msg.sender], "NOT_EXECUTOR");
        _;
    }

    modifier onlyRouterManager() {
        require(routerManagers[msg.sender] || msg.sender == owner, "NOT_ROUTER_MANAGER");
        _;
    }

    modifier whenNotPaused() {
        require(!paused, "PAUSED");
        _;
    }

    modifier nonReentrant() {
        require(unlocked == 1, "REENTRANT");
        unlocked = 2;
        _;
        unlocked = 1;
    }

    constructor(address admin) {
        require(admin != address(0), "ZERO_ADMIN");
        owner = admin;
        withdrawExecutors[admin] = true;
        routerManagers[admin] = true;
    }

    function setWithdrawExecutor(address account, bool allowed) external onlyOwner {
        withdrawExecutors[account] = allowed;
        emit WithdrawExecutorUpdated(account, allowed, msg.sender);
    }

    function setRouterManager(address account, bool allowed) external onlyOwner {
        routerManagers[account] = allowed;
        emit RouterManagerUpdated(account, allowed, msg.sender);
    }

    function setRouterAllowed(address router, bool allowed) external onlyRouterManager {
        allowedRouters[router] = allowed;
        emit RouterAllowedUpdated(router, allowed, msg.sender);
    }

    function setTokenAllowed(address token, bool allowed) external onlyOwner {
        allowedTokens[token] = allowed;
        emit TokenAllowedUpdated(token, allowed, msg.sender);
    }

    function pause() external onlyOwner {
        paused = true;
        emit Paused(msg.sender);
    }

    function unpause() external onlyOwner {
        paused = false;
        emit Unpaused(msg.sender);
    }

    function withdraw(address token, address to, uint256 amount, bytes32 withdrawId) external onlyWithdrawExecutor whenNotPaused nonReentrant {
        require(allowedTokens[token], "TOKEN_NOT_ALLOWED");
        require(to != address(0), "ZERO_TO");
        require(!processedWithdrawIds[withdrawId], "WITHDRAW_ALREADY_PROCESSED");
        processedWithdrawIds[withdrawId] = true;
        require(IERC20Minimal(token).transfer(to, amount), "TRANSFER_FAILED");
        emit WithdrawExecuted(withdrawId, token, to, amount, msg.sender);
    }

    function rescueToken(address token, address to, uint256 amount, bytes32 rescueId) external onlyOwner nonReentrant {
        require(to != address(0), "ZERO_TO");
        require(!allowedTokens[token], "TOKEN_RESCUE_BLOCKED");
        require(IERC20Minimal(token).transfer(to, amount), "TRANSFER_FAILED");
        emit RescueExecuted(rescueId, token, to, amount, msg.sender);
    }
}
