// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

interface IERC20Forwarder {
    function balanceOf(address account) external view returns (uint256);
    function transfer(address to, uint256 amount) external returns (bool);
}

contract DepositRouter {
    uint256 public immutable userId;
    address public immutable vault;
    address public immutable token;

    event DepositForwarded(uint256 indexed userId, address indexed token, uint256 amount, address indexed from, address vault);

    constructor(uint256 userId_, address vault_, address token_) {
        require(vault_ != address(0), "ZERO_VAULT");
        require(token_ != address(0), "ZERO_TOKEN");
        userId = userId_;
        vault = vault_;
        token = token_;
    }

    function forward() external {
        forwardToken(token);
    }

    function forwardToken(address token_) public {
        require(token_ == token, "TOKEN_NOT_ALLOWED");
        uint256 amount = IERC20Forwarder(token_).balanceOf(address(this));
        require(amount > 0, "NO_BALANCE");
        require(IERC20Forwarder(token_).transfer(vault, amount), "TRANSFER_FAILED");
        emit DepositForwarded(userId, token_, amount, msg.sender, vault);
    }

    function sweepNative(address payable to) external {
        require(to != address(0), "ZERO_TO");
        uint256 amount = address(this).balance;
        (bool ok,) = to.call{value: amount}("");
        require(ok, "SWEEP_FAILED");
    }

    receive() external payable {}
}
